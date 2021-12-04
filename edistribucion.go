// Porting https://github.com/trocotronic/edistribucion to Go

package edistribucion

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gocolly/colly"
	"github.com/gocolly/colly/storage"
)

const dashboardURL = "https://zonaprivada.edistribucion.com/areaprivada/s/sfsites/aura?"

var auraConfig AuraConfig

type Context struct {
	Mode       string `json:"mode"`
	App        string `json:"app"`
	Fwuid      string `json:"fwuid"`
	Loaded     Loaded `json:"loaded"`
	Apce       uint   `json:"apce"`
	Apck       string `json:"apck"`
	Mlr        uint   `json:"mlr"`
	PathPrefix string `json:"pathPrefix"`
	Dns        string `json:"dns"`
	Ls         uint   `json:"ls"`
}

type Loaded struct {
	Token string `json:"APPLICATION@markup://siteforce:loginApp2"`
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
	Events []Event `json:"events"`
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
	"getLoginInfo": Action{
		ID:                215,
		Descriptor:        "WP_Monitor_CTRL/ACTION$getLoginInfo",
		CallingDescriptor: "WP_Monitor",
		Params:            map[string]string{"serviceNumber": "S011"},
	},
	"getCups": Action{
		ID:                270,
		Descriptor:        "WP_ContadorICP_F2_CTRL/ACTION$getCUPSReconectarICP",
		CallingDescriptor: "WP_Reconnect_ICP",
		Params:            map[string]string{"visSelected": ""},
	},
	"getMeter": Action{
		ID:                294,
		Descriptor:        "WP_ContadorICP_F2_CTRL/ACTION$consultarContador",
		CallingDescriptor: "WP_Reconnect_Detail",
		Params:            map[string]string{"cupsId": ""},
	},
}

type Client struct {
	username  string
	password  string
	collector *colly.Collector
	ctx       *Context
	accountID string
	Debug     bool
}

func (c *Client) MeterInfo(cupsID string) (*MeterInfo, error) {
	var met MeterActionResponse
	getMeter := actions["getMeter"]
	getMeter.Params["cupsId"] = cupsID
	err := c.sendAction(getMeter, "WP_ContadorICP_F2_CTRL.consultarContador", &met)
	if err != nil {
		return nil, err
	}

	rv := met.Actions[0].MeterReturnValue
	if rv.HasWarning {
		return nil, errors.New(rv.Warning.Message)
	}

	return &met.Actions[0].MeterReturnValue.Data, nil
}

func NewClient(username, password string) *Client {
	c := colly.NewCollector()
	c.SetRequestTimeout(90 * time.Second)
	c.UserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/70.0.3538.77 Safari/537.36"
	return &Client{username: username, password: password, collector: c}
}

func (cl *Client) Login() error {
	c := cl.collector
	c.SetStorage(&storage.InMemoryStorage{})
	cl.ctx = &Context{}
	c.OnHTML("script", func(e *colly.HTMLElement) {
		path := e.Attr("src")
		if regexp.MustCompile(`resources.js`).MatchString(path) {
			j := strings.Split(path, "/")[5]
			decoded, err := url.QueryUnescape(j)
			if err != nil {
				panic(err)
			}

			//var data map[string]string
			err = json.Unmarshal([]byte(decoded), cl.ctx)
			if err != nil {
				panic(err)
			}

		}
	})

	c.Visit("https://zonaprivada.edistribucion.com/areaprivada/s/login?ec=302&startURL=%2Fareaprivada%2Fs%2F")
	params := map[string]string{
		"username": cl.username,
		"password": cl.password,
		"startUrl": "/areaprivada/s/",
	}

	action := Action{
		ID:                91,
		Descriptor:        "LightningLoginFormController/ACTION$login",
		CallingDescriptor: "WP_LoginForm",
		Params:            params,
	}

	msg := Message{[]Action{action}}

	//d := Data{Message: msg, Context: ctx, Token: "undefined", PageURI: "/areaprivada/s/login/?language=es&startURL=%2Fareaprivada%2Fs%2F&ec=302"}
	msgm, err := json.Marshal(msg)
	if err != nil {
		panic(err)
	}
	ctxm, err := json.Marshal(cl.ctx)
	if err != nil {
		panic(err)
	}

	d := map[string]string{
		"message":      string(msgm),
		"aura.context": string(ctxm),
		"aura.pageURI": "/areaprivada/s/login/?language=es&startURL=%2Fareaprivada%2Fs%2F&ec=302",
		"aura.token":   "undefined",
	}

	var response LoginResponse
	c.OnResponse(func(r *colly.Response) {
		json.Unmarshal(r.Body, &response)
	})

	err = c.Post("https://zonaprivada.edistribucion.com/areaprivada/s/sfsites/aura?other.LightningLoginForm.login=1", d)
	if err != nil {
		return err
	}

	if len(response.Events) == 0 {
		return errors.New("invalid login response")
	}

	c = c.Clone()
	c.OnResponse(func(r *colly.Response) {
		// TODO: add debugging
		//fmt.Println(string(r.Body))
	})

	// Follow the redirect after login
	err = c.Visit(response.Events[0].Attributes.Values.Url)
	if err != nil {
		return err
	}

	c = c.Clone()
	c.OnResponse(func(r *colly.Response) {
		p := regexpGroups(`(?P<auraConfig>var auraConfig = )(?P<json>.*?);`, string(r.Body))
		json.Unmarshal([]byte(p["json"]), &auraConfig)
	})

	// Landing page
	err = c.Visit("https://zonaprivada.edistribucion.com/areaprivada/s/")
	if err != nil {
		return err
	}

	// Get login info
	var ar GetLoginActionResponse
	err = cl.sendAction(actions["getLoginInfo"], "WP_Monitor_CTRL.getLoginInfo", &ar)
	if err != nil {
		return err
	}

	cl.accountID = ar.Actions[0].ReturnValue.Visibility.ID

	return nil
}

func (c *Client) ListCups() ([]Cups, error) {
	var gc CupsActionResponse
	getCups := actions["getCups"]
	getCups.Params["visSelected"] = c.accountID
	err := c.sendAction(getCups, "WP_ContadorICP_F2_CTRL.getCUPSReconectarICP", &gc)
	if err != nil {
		return nil, err
	}

	return gc.Actions[0].CupsReturnValue.Data.Cups, nil
}

func (client *Client) sendAction(action Action, command string, actionResponse interface{}) error {
	ctx := client.ctx
	c := client.collector.Clone()
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
		"aura.pageURI": "/areaprivada/s/wp-online-access",
		"aura.token":   auraConfig.Token,
	}
	c.OnRequest(func(r *colly.Request) {
		r.Headers.Set("Accept", "application/json")
	})

	c.OnResponse(func(r *colly.Response) {
		if strings.HasPrefix(r.Headers.Get("Content-Type"), "application/json") {
			if client.Debug {
				fmt.Println(string(r.Body))
			}
			json.Unmarshal(r.Body, actionResponse)
		}
	})

	err = c.Post(dashboardURL+command, d)
	if err != nil {
		return err
	}

	return nil
}

func regexpGroups(regEx, url string) (paramsMap map[string]string) {
	var compRegEx = regexp.MustCompile(regEx)
	match := compRegEx.FindStringSubmatch(url)

	paramsMap = make(map[string]string)
	for i, name := range compRegEx.SubexpNames() {
		if i > 0 && i <= len(match) {
			paramsMap[name] = match[i]
		}
	}
	return paramsMap
}
