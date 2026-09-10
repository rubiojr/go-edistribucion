package edistribucion

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAuraContext(t *testing.T) {
	rawContext := `{"mode":"PROD","app":"siteforce:communityApp","fwuid":"framework-id","loaded":{"APPLICATION@markup://siteforce:communityApp":"community-token","COMPONENT@markup://c:WP_Monitor":"component-token"},"apce":1,"pathPrefix":"/areaprivada"}`
	page := fmt.Appendf(nil, `<html><script src="/areaprivada/s/sfsites/l/%s/resources.js?rv=1"></script></html>`, url.PathEscape(rawContext))

	ctx, err := parseAuraContext(page)
	require.NoError(t, err)

	encoded, err := json.Marshal(ctx)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"mode":"PROD",
		"app":"siteforce:communityApp",
		"fwuid":"framework-id",
		"loaded":{
			"APPLICATION@markup://siteforce:communityApp":"community-token",
			"COMPONENT@markup://c:WP_Monitor":"component-token"
		},
		"dn":[],
		"globals":{},
		"uad":false
	}`, string(encoded))
}

func TestParseAuraContextErrors(t *testing.T) {
	tests := []struct {
		name string
		page string
		err  string
	}{
		{name: "missing script", page: `<html></html>`, err: "resources.js context not found"},
		{name: "invalid context", page: `<script src="/%7Bbad%7D/resources.js"></script>`, err: "decode Aura context"},
		{name: "incomplete context", page: `<script src="/%7B%22mode%22%3A%22PROD%22%7D/resources.js"></script>`, err: "aura context is incomplete"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseAuraContext([]byte(tt.page))
			assert.ErrorContains(t, err, tt.err)
		})
	}
}

func TestAuraToken(t *testing.T) {
	for _, property := range []string{"eikoocnekot", "eikooknekot"} {
		t.Run(property, func(t *testing.T) {
			client := NewClient("", "")
			require.NoError(t, client.collector.SetCookies(siteURL, []*http.Cookie{{
				Name:  "aura-token-cookie",
				Value: "secret-token",
			}}))

			page := fmt.Appendf(nil, `<script>var auraConfig={%s:"aura-token-cookie"};</script>`, property)
			token, err := client.auraToken(page)
			require.NoError(t, err)
			assert.Equal(t, "secret-token", token)
		})
	}
}

func TestLoginResponseError(t *testing.T) {
	err := loginResponseError(LoginResponse{Actions: []ActionOutcome{{
		State:       "SUCCESS",
		ReturnValue: json.RawMessage(`"Usuario o contraseña no válidos"`),
	}}})

	assert.ErrorContains(t, err, "Usuario o contraseña no válidos")
}

func TestValidateActionResponse(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr string
	}{
		{name: "success", body: `{"actions":[{"state":"SUCCESS"}]}`},
		{name: "action error", body: `{"actions":[{"state":"ERROR","error":[{"message":"expired session"}]}]}`, wantErr: "expired session"},
		{name: "missing actions", body: `{}`, wantErr: "contains no actions"},
		{name: "invalid JSON", body: `{`, wantErr: "decode action response"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateActionResponse([]byte(tt.body))
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestActionWithParamDoesNotMutateTemplate(t *testing.T) {
	template := Action{Params: map[string]string{"cupsId": ""}}
	action := actionWithParam(template, "cupsId", "cups-1")

	assert.Empty(t, template.Params["cupsId"])
	assert.Equal(t, "cups-1", action.Params["cupsId"])
}

func TestLoginFlow(t *testing.T) {
	client := NewClient("user@example.com", "password")
	requestNumber := 0
	client.collector.WithTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestNumber++
		switch requestNumber {
		case 1:
			assert.Equal(t, siteURL, req.URL.String())
			return testResponse(req, `<html>session proxy</html>`, nil), nil
		case 2:
			assert.Equal(t, loginURL, req.URL.String())
			return testResponse(req, auraPage("siteforce:loginApp2", "login-component", ""), nil), nil
		case 3:
			assert.Equal(t, "POST", req.Method)
			assert.Equal(t, "r=1&other.LightningLoginForm.login=1", req.URL.RawQuery)
			require.NoError(t, req.ParseForm())
			assert.Equal(t, "null", req.Form.Get("aura.token"))
			assert.Contains(t, req.Form.Get("message"), `"descriptor":"apex://LightningLoginFormController/ACTION$login"`)
			body := fmt.Sprintf(`{"events":[{"attributes":{"values":{"url":%q}}}]}`, siteURL+"/frontdoor")
			return testResponse(req, body, nil), nil
		case 4:
			assert.Equal(t, "/areaprivada/s/frontdoor", req.URL.Path)
			return testResponse(req, `<script>window.location.replace("/areaprivada/s/session")</script>`, nil), nil
		case 5:
			assert.Equal(t, "/areaprivada/s/session", req.URL.Path)
			return testResponse(req, `<html>authenticated</html>`, nil), nil
		case 6:
			assert.Equal(t, siteURL, req.URL.String())
			headers := http.Header{"Set-Cookie": {"aura-cookie=secret-token; Path=/areaprivada/s; Secure"}}
			return testResponse(req, auraPage("siteforce:communityApp", "community-component", "aura-cookie"), headers), nil
		case 7:
			assert.Equal(t, "r=0&other.WP_Monitor_CTRL.getLoginInfo=1", req.URL.RawQuery)
			require.NoError(t, req.ParseForm())
			assert.Equal(t, "secret-token", req.Form.Get("aura.token"))
			assert.Contains(t, req.Form.Get("aura.context"), "community-component")
			return testResponse(req, `{"actions":[{"id":"215","state":"SUCCESS","returnValue":{"Id":"user-id","Name":"Test User","visibility":{"Id":"account-id"}}}]}`, nil), nil
		case 8:
			assert.Equal(t, "r=1&other.WP_ContadorICP_F2_CTRL.getCUPSReconectarICP=1", req.URL.RawQuery)
			require.NoError(t, req.ParseForm())
			assert.Contains(t, req.Form.Get("message"), `"visSelected":"account-id"`)
			return testResponse(req, `{"actions":[{"id":"270","state":"SUCCESS","returnValue":{"data":{"lstCups":[{"Id":"cups-id","Name":"ES001"}]}}}]}`, nil), nil
		case 9:
			assert.Equal(t, "r=2&other.WP_ContadorICP_F2_CTRL.consultarContador2=1", req.URL.RawQuery)
			require.NoError(t, req.ParseForm())
			assert.Contains(t, req.Form.Get("message"), `"id":522`)
			assert.Contains(t, req.Form.Get("message"), `ACTION$consultarContador2`)
			return testResponse(req, `{"actions":[{"id":"522","state":"SUCCESS","returnValue":{"data":{"potenciaActual":1.2,"potenciaContratada":4.6,"percent":"26%","estadoICP":"Conectado","totalizador":"123"}}}]}`, nil), nil
		default:
			return nil, fmt.Errorf("unexpected request %d to %s", requestNumber, req.URL)
		}
	}))

	require.NoError(t, client.Login())
	assert.Equal(t, "account-id", client.accountID)
	cups, err := client.ListCups()
	require.NoError(t, err)
	require.Len(t, cups, 1)
	assert.Equal(t, "cups-id", cups[0].Id)
	meter, err := client.MeterInfo(cups[0].Id)
	require.NoError(t, err)
	assert.Equal(t, 1.2, meter.PotenciaActual)
	assert.Equal(t, 9, requestNumber)
}

func TestValidateRemoteURL(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		wantErr string
	}{
		{name: "edistribucion", rawURL: siteURL},
		{name: "Salesforce", rawURL: "https://tenant.my.salesforce.com/frontdoor"},
		{name: "foreign host", rawURL: "https://example.com/", wantErr: "unexpected host"},
		{name: "insecure", rawURL: "http://zonaprivada.edistribucion.com/", wantErr: "non-HTTPS"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remoteURL, err := url.Parse(tt.rawURL)
			require.NoError(t, err)
			err = validateRemoteURL(remoteURL)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestVisitRejectsForeignJavaScriptRedirect(t *testing.T) {
	client := NewClient("", "")
	requests := 0
	client.collector.WithTransport(roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		return testResponse(req, `<script>window.location.replace("//internal.example/")</script>`, nil), nil
	}))

	_, err := client.visit(siteURL)
	assert.ErrorContains(t, err, "unexpected host")
	assert.Equal(t, 1, requests)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testResponse(req *http.Request, body string, headers http.Header) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     headers,
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func auraPage(app, loadedValue, tokenCookie string) string {
	context := fmt.Sprintf(`{"mode":"PROD","app":%q,"fwuid":"framework-id","loaded":{"APPLICATION@markup://%s":%q}}`, app, app, loadedValue)
	page := fmt.Sprintf(`<script src="/areaprivada/s/sfsites/l/%s/resources.js"></script>`, url.PathEscape(context))
	if tokenCookie != "" {
		page += fmt.Sprintf(`<script>var auraConfig={eikoocnekot:%q};</script>`, tokenCookie)
	}
	return page
}
