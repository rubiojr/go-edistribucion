// Porting https://github.com/trocotronic/edistribucion to Go

package edistribucion

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/gocolly/colly/v2/storage"
)

const (
	hostURL      = "https://zonaprivada.edistribucion.com"
	siteURL      = hostURL + "/areaprivada/s"
	loginURL     = siteURL + "/login?ec=302&startURL=%2Fareaprivada%2Fs%2F"
	dashboardURL = siteURL + "/sfsites/aura?"
)

var (
	resourcesScriptPattern = regexp.MustCompile(`(?i)(?:src|data-href)=["']([^"']*resources\.js[^"']*)`)
	javascriptRedirect     = regexp.MustCompile(`window\.location\.replace\(["']([^"']+)["']\)`)
	tokenCookiePattern     = regexp.MustCompile(`["']?eikoo[ck]nekot["']?\s*:\s*["']([^"']+)["']`)
)

type Context struct {
	Mode       string         `json:"mode"`
	App        string         `json:"app"`
	Fwuid      string         `json:"fwuid"`
	Loaded     Loaded         `json:"loaded"`
	DN         []any          `json:"dn"`
	Globals    map[string]any `json:"globals"`
	UAD        bool           `json:"uad"`
	Apce       uint           `json:"apce,omitempty"`
	Apck       string         `json:"apck,omitempty"`
	Mlr        uint           `json:"mlr,omitempty"`
	PathPrefix string         `json:"pathPrefix,omitempty"`
	Dns        string         `json:"dns,omitempty"`
	Ls         uint           `json:"ls,omitempty"`
}

type Loaded struct {
	Token  string `json:"APPLICATION@markup://siteforce:loginApp2,omitempty"`
	values map[string]string
}

func (l *Loaded) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &l.values); err != nil {
		return err
	}
	l.Token = l.values["APPLICATION@markup://siteforce:loginApp2"]
	return nil
}

func (l Loaded) MarshalJSON() ([]byte, error) {
	if l.values != nil {
		return json.Marshal(l.values)
	}
	return json.Marshal(map[string]string{
		"APPLICATION@markup://siteforce:loginApp2": l.Token,
	})
}

type Action struct {
	ID                uint              `json:"id"`
	Descriptor        string            `json:"descriptor"`
	CallingDescriptor string            `json:"callingDescriptor"`
	Params            map[string]string `json:"params"`
}

type Data struct {
	Message Message `json:"message"`
	Context Context `json:"aura.context"`
	PageURI string  `json:"aura.pageURI"`
	Token   string  `json:"aura.token"`
}

type Message struct {
	Actions []Action `json:"actions"`
}

type LoginResponse struct {
	Events  []Event         `json:"events"`
	Actions []ActionOutcome `json:"actions"`
}

type ActionOutcome struct {
	State       string          `json:"state"`
	ReturnValue json.RawMessage `json:"returnValue"`
	Error       []ActionError   `json:"error"`
}

type ActionError struct {
	Message string `json:"message"`
}

type Event struct {
	Descriptor string    `json:"descriptor"`
	Attributes Attribute `json:"attributes"`
}

type Attribute struct {
	Values Value `json:"values"`
}

type Value struct {
	Url string `json:"url"`
}

type AuraConfig struct {
	Token string `json:"token"`
}

type ActionsResponse struct {
	Context Context `json:"context"`
}

type GetLoginActionResponse struct {
	ActionsResponse
	Actions []GetLoginResponse `json:"actions"`
}

type GetLoginResponse struct {
	ID          string              `json:"id"`
	State       string              `json:"state"`
	ReturnValue GetLoginReturnValue `json:"returnValue"`
}

type GetLoginReturnValue struct {
	ID         string     `json:"Id"`
	Name       string     `json:"Name"`
	FirstName  string     `json:"firstName"`
	Visibility Visibility `json:"visibility"`
}

type Visibility struct {
	ID string `json:"Id"`
}

type CupsActionResponse struct {
	ActionsResponse
	Actions []CupsResponse `json:"actions"`
}

type Cups struct {
	Id                  string `json:"Id"`
	Name                string `json:"Name"`
	ProvisioningAddress string `json:"Provisioning_address__c"`
	ButonLink           string `json:"ButtonLink"`
}

type CupsList struct {
	Cups []Cups `json:"lstCups"`
}

type CupsResponse struct {
	Id              string          `json:"Id"`
	State           string          `json:"state"`
	CupsReturnValue CupsReturnValue `json:"returnValue"`
}

type CupsReturnValue struct {
	Data CupsList `json:"data"`
}

type MeterActionResponse struct {
	ActionsResponse
	Actions []MeterResponse `json:"actions"`
}

type MeterResponse struct {
	Id               string           `json:"Id"`
	State            string           `json:"state"`
	MeterReturnValue MeterReturnValue `json:"returnValue"`
}

type MeterReturnValue struct {
	Data          MeterInfo    `json:"data"`
	HasWarning    bool         `json:"hasWarning"`
	RejectPromise bool         `json:"rejectPromise"`
	Warning       MeterWarning `json:"warning"`
}

type MeterInfo struct {
	PotenciaActual     float64 `json:"potenciaActual"`
	Totalizador        string  `json:"totalizador"`
	EstadoICP          string  `json:"estadoICP"`
	PotenciaContratada float64 `json:"potenciaContratada"`
	Percentage         string  `json:"percent"`
}

type MeterWarning struct {
	Message string `json:"message"`
}

var actions = map[string]Action{
	"getLoginInfo": {
		ID:                215,
		Descriptor:        "apex://WP_Monitor_CTRL/ACTION$getLoginInfo",
		CallingDescriptor: "markup://c:WP_Monitor",
		Params:            map[string]string{"serviceNumber": "S011"},
	},
	"getCups": {
		ID:                270,
		Descriptor:        "apex://WP_ContadorICP_F2_CTRL/ACTION$getCUPSReconectarICP",
		CallingDescriptor: "markup://c:WP_Reconnect_ICP",
		Params:            map[string]string{"visSelected": ""},
	},
	"getMeter": {
		ID:                522,
		Descriptor:        "apex://WP_ContadorICP_F2_CTRL/ACTION$consultarContador2",
		CallingDescriptor: "markup://c:WP_Reconnect_Detail",
		Params:            map[string]string{"cupsId": ""},
	},
}

type Client struct {
	username  string
	password  string
	collector *colly.Collector
	ctx       *Context
	token     string
	accountID string
	requestID atomic.Uint64
	Debug     bool
}

func (c *Client) MeterInfo(cupsID string) (*MeterInfo, error) {
	var met MeterActionResponse
	getMeter := actionWithParam(actions["getMeter"], "cupsId", cupsID)
	err := c.sendAction(getMeter, "WP_ContadorICP_F2_CTRL.consultarContador2", &met)
	if err != nil {
		return nil, err
	}
	if len(met.Actions) == 0 {
		return nil, errors.New("meter response contains no actions")
	}

	rv := met.Actions[0].MeterReturnValue
	if rv.HasWarning {
		return nil, errors.New(rv.Warning.Message)
	}

	return &met.Actions[0].MeterReturnValue.Data, nil
}

func NewClient(username, password string) *Client {
	c := colly.NewCollector()
	c.AllowURLRevisit = true
	c.SetRequestTimeout(90 * time.Second)
	c.SetRedirectHandler(func(req *http.Request, _ []*http.Request) error {
		return validateRemoteURL(req.URL)
	})
	c.UserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"
	return &Client{username: username, password: password, collector: c}
}

func (cl *Client) Login() error {
	if err := cl.collector.SetStorage(&storage.InMemoryStorage{}); err != nil {
		return fmt.Errorf("initialize session storage: %w", err)
	}
	cl.ctx = nil
	cl.token = ""
	cl.accountID = ""
	cl.requestID.Store(0)

	// Salesforce initializes the session proxy on the site root before login.
	if _, err := cl.visit(siteURL); err != nil {
		return fmt.Errorf("initialize login session: %w", err)
	}
	loginPage, err := cl.visit(loginURL)
	if err != nil {
		return fmt.Errorf("load login page: %w", err)
	}
	cl.ctx, err = parseAuraContext(loginPage)
	if err != nil {
		return fmt.Errorf("load login context: %w", err)
	}

	response, err := cl.submitLogin()
	if err != nil {
		return err
	}
	if len(response.Events) == 0 {
		return loginResponseError(response)
	}
	if _, err := cl.visit(response.Events[0].Attributes.Values.Url); err != nil {
		return fmt.Errorf("follow login redirect: %w", err)
	}

	landingPage, err := cl.visit(siteURL)
	if err != nil {
		return fmt.Errorf("load landing page: %w", err)
	}
	cl.ctx, err = parseAuraContext(landingPage)
	if err != nil {
		return fmt.Errorf("load authenticated context: %w", err)
	}
	cl.token, err = cl.auraToken(landingPage)
	if err != nil {
		return err
	}

	// Get login info
	var ar GetLoginActionResponse
	err = cl.sendAction(actions["getLoginInfo"], "WP_Monitor_CTRL.getLoginInfo", &ar)
	if err != nil {
		return err
	}

	if len(ar.Actions) == 0 {
		return errors.New("login info response contains no actions")
	}
	cl.accountID = ar.Actions[0].ReturnValue.Visibility.ID
	if cl.accountID == "" {
		return errors.New("login info response contains no account ID")
	}

	return nil
}

func (cl *Client) submitLogin() (LoginResponse, error) {
	action := Action{
		ID:                91,
		Descriptor:        "apex://LightningLoginFormController/ACTION$login",
		CallingDescriptor: "markup://c:WP_LoginForm",
		Params: map[string]string{
			"username": cl.username,
			"password": cl.password,
			"startUrl": "/areaprivada/s/",
		},
	}
	msg, err := json.Marshal(Message{Actions: []Action{action}})
	if err != nil {
		return LoginResponse{}, fmt.Errorf("encode login message: %w", err)
	}
	ctx, err := json.Marshal(cl.ctx)
	if err != nil {
		return LoginResponse{}, fmt.Errorf("encode login context: %w", err)
	}
	data := map[string]string{
		"message":      string(msg),
		"aura.context": string(ctx),
		"aura.pageURI": "/areaprivada/s/login/?language=es&startURL=%2Fareaprivada%2Fs%2F&ec=302",
		"aura.token":   "null",
	}
	body, err := cl.post(dashboardURL+"r=1&other.LightningLoginForm.login=1", data)
	if err != nil {
		return LoginResponse{}, fmt.Errorf("submit login: %w", err)
	}

	var response LoginResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return LoginResponse{}, fmt.Errorf("decode login response: %w", err)
	}
	return response, nil
}

func (c *Client) ListCups() ([]Cups, error) {
	var gc CupsActionResponse
	getCups := actionWithParam(actions["getCups"], "visSelected", c.accountID)
	err := c.sendAction(getCups, "WP_ContadorICP_F2_CTRL.getCUPSReconectarICP", &gc)
	if err != nil {
		return nil, err
	}
	if len(gc.Actions) == 0 {
		return nil, errors.New("CUPS response contains no actions")
	}

	return gc.Actions[0].CupsReturnValue.Data.Cups, nil
}

func (client *Client) sendAction(action Action, command string, actionResponse interface{}) error {
	if client.ctx == nil || client.token == "" {
		return errors.New("client is not logged in")
	}
	ctx := client.ctx
	msg := Message{[]Action{action}}
	msgm, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	ctxm, err := json.Marshal(ctx)
	if err != nil {
		return err
	}

	d := map[string]string{
		"message":      string(msgm),
		"aura.context": string(ctxm),
		"aura.pageURI": "/areaprivada/s/",
		"aura.token":   client.token,
	}
	requestID := client.requestID.Add(1) - 1
	body, err := client.post(fmt.Sprintf("%sr=%d&other.%s=1", dashboardURL, requestID, command), d)
	if err != nil {
		return err
	}
	if err := validateActionResponse(body); err != nil {
		return err
	}
	if err := json.Unmarshal(body, actionResponse); err != nil {
		return fmt.Errorf("decode action response: %w", err)
	}

	return nil
}

func actionWithParam(action Action, key, value string) Action {
	params := make(map[string]string, len(action.Params))
	for name, param := range action.Params {
		params[name] = param
	}
	params[key] = value
	action.Params = params
	return action
}

func (client *Client) post(rawURL string, data map[string]string) ([]byte, error) {
	c := client.collector.Clone()
	var body []byte
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("Accept", "application/json")
	})
	c.OnResponse(func(r *colly.Response) {
		body = append(body[:0], r.Body...)
	})
	if err := c.Post(rawURL, data); err != nil {
		return nil, err
	}
	if client.Debug {
		fmt.Println(string(body))
	}
	return body, nil
}

func (client *Client) visit(rawURL string) ([]byte, error) {
	const maxJavaScriptRedirects = 5
	for range maxJavaScriptRedirects + 1 {
		current, err := url.Parse(rawURL)
		if err != nil {
			return nil, err
		}
		if err := validateRemoteURL(current); err != nil {
			return nil, err
		}
		c := client.collector.Clone()
		var body []byte
		c.OnResponse(func(r *colly.Response) {
			body = append(body[:0], r.Body...)
		})
		if err := c.Visit(rawURL); err != nil {
			return nil, err
		}

		redirect := javascriptRedirect.FindSubmatch(body)
		if redirect == nil {
			return body, nil
		}
		next, err := current.Parse(html.UnescapeString(string(redirect[1])))
		if err != nil {
			return nil, fmt.Errorf("parse JavaScript redirect: %w", err)
		}
		rawURL = next.String()
	}
	return nil, errors.New("too many JavaScript redirects")
}

func validateRemoteURL(remoteURL *url.URL) error {
	if remoteURL.Scheme != "https" {
		return fmt.Errorf("refusing non-HTTPS URL %q", remoteURL.String())
	}
	host := strings.ToLower(remoteURL.Hostname())
	if host == "zonaprivada.edistribucion.com" ||
		strings.HasSuffix(host, ".salesforce.com") ||
		strings.HasSuffix(host, ".force.com") ||
		strings.HasSuffix(host, ".salesforce-sites.com") ||
		strings.HasSuffix(host, ".salesforce-experience.com") {
		return nil
	}
	return fmt.Errorf("refusing URL on unexpected host %q", host)
}

func parseAuraContext(page []byte) (*Context, error) {
	match := resourcesScriptPattern.FindSubmatch(page)
	if match == nil {
		return nil, errors.New("resources.js context not found")
	}
	decoded, err := url.PathUnescape(html.UnescapeString(string(match[1])))
	if err != nil {
		return nil, fmt.Errorf("decode resources.js URL: %w", err)
	}
	resourceIndex := strings.Index(decoded, "/resources.js")
	if resourceIndex < 0 {
		return nil, errors.New("invalid resources.js URL")
	}
	encodedContext := decoded[:resourceIndex]
	start := strings.IndexByte(encodedContext, '{')
	end := strings.LastIndexByte(encodedContext, '}')
	if start < 0 || end < start {
		return nil, errors.New("aura context JSON not found")
	}

	var bootstrap Context
	if err := json.Unmarshal([]byte(encodedContext[start:end+1]), &bootstrap); err != nil {
		return nil, fmt.Errorf("decode Aura context: %w", err)
	}
	if bootstrap.Mode == "" || bootstrap.App == "" || bootstrap.Fwuid == "" || bootstrap.Loaded.values == nil {
		return nil, errors.New("aura context is incomplete")
	}
	bootstrap.DN = []any{}
	bootstrap.Globals = map[string]any{}
	bootstrap.UAD = false
	bootstrap.Apce = 0
	bootstrap.Apck = ""
	bootstrap.Mlr = 0
	bootstrap.PathPrefix = ""
	bootstrap.Dns = ""
	bootstrap.Ls = 0
	return &bootstrap, nil
}

func (client *Client) auraToken(page []byte) (string, error) {
	match := tokenCookiePattern.FindSubmatch(page)
	if match == nil {
		return "", errors.New("aura token cookie name not found")
	}
	name := string(match[1])
	for _, cookie := range client.collector.Cookies(siteURL) {
		if cookie.Name == name && cookie.Value != "" {
			return cookie.Value, nil
		}
	}
	return "", fmt.Errorf("aura token cookie %q not found", name)
}

func loginResponseError(response LoginResponse) error {
	for _, action := range response.Actions {
		for _, actionErr := range action.Error {
			if actionErr.Message != "" {
				return fmt.Errorf("login failed: %s", actionErr.Message)
			}
		}
		var message string
		if json.Unmarshal(action.ReturnValue, &message) == nil && message != "" {
			return fmt.Errorf("login failed: %s", message)
		}
	}
	return errors.New("login response contains no redirect event")
}

func validateActionResponse(body []byte) error {
	var response struct {
		Actions []ActionOutcome `json:"actions"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("decode action response: %w", err)
	}
	if len(response.Actions) == 0 {
		return errors.New("action response contains no actions")
	}
	for _, action := range response.Actions {
		if action.State == "SUCCESS" {
			continue
		}
		for _, actionErr := range action.Error {
			if actionErr.Message != "" {
				return fmt.Errorf("action failed: %s", actionErr.Message)
			}
		}
		return fmt.Errorf("action failed with state %q", action.State)
	}
	return nil
}
