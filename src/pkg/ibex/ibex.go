package ibex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Ibex struct {
	address  string
	authUser string
	authPass string
	timeout  time.Duration
	method   string
	urlPath  string
	inValue  interface{}
	outPtr   interface{}
	headers  map[string]string
	queries  map[string][]string
}

func New(addr, user, pass string, timeout int64) *Ibex {
	if !strings.HasPrefix(addr, "http") {
		addr = "http://" + addr
	}

	return &Ibex{
		address:  addr,
		authUser: user,
		authPass: pass,
		timeout:  time.Duration(timeout) * time.Millisecond,
		headers:  make(map[string]string),
		queries:  make(map[string][]string),
	}
}

func (i *Ibex) In(v interface{}) *Ibex {
	i.inValue = v
	return i
}

func (i *Ibex) Out(ptr interface{}) *Ibex {
	i.outPtr = ptr
	return i
}

func (i *Ibex) Path(p string) *Ibex {
	i.urlPath = p
	return i
}

func (i *Ibex) Method(m string) *Ibex {
	i.method = strings.ToUpper(m)
	return i
}

func (i *Ibex) Header(key, value string) *Ibex {
	i.headers[key] = value
	return i
}

func (i *Ibex) QueryString(key, value string) *Ibex {
	if param, ok := i.queries[key]; ok {
		i.queries[key] = append(param, value)
	} else {
		i.queries[key] = []string{value}
	}
	return i
}

func (i *Ibex) buildUrl() {
	var queries string
	if len(i.queries) > 0 {
		var buf bytes.Buffer
		for k, v := range i.queries {
			for _, vv := range v {
				buf.WriteString(url.QueryEscape(k))
				buf.WriteByte('=')
				buf.WriteString(url.QueryEscape(vv))
				buf.WriteByte('&')
			}
		}
		queries = buf.String()
		queries = queries[0 : len(queries)-1]
	}

	if len(queries) > 0 {
		if strings.Contains(i.urlPath, "?") {
			i.urlPath += "&" + queries
		} else {
			i.urlPath = i.urlPath + "?" + queries
		}
	}
}

func (i *Ibex) do() error {
	i.buildUrl()

	var req *http.Request
	var err error
	var bs []byte

	if i.inValue != nil {
		bs, err = json.Marshal(i.inValue)
		if err != nil {
			return err
		}
		req, err = http.NewRequest(i.method, i.address+i.urlPath, bytes.NewBuffer(bs))
	} else {
		req, err = http.NewRequest(i.method, i.address+i.urlPath, nil)
	}

	if err != nil {
		return err
	}

	for key, value := range i.headers {
		req.Header.Set(key, value)
	}

	if i.authUser != "" {
		req.SetBasicAuth(i.authUser, i.authPass)
	}

	if i.method != http.MethodGet {
		req.Header.Set("Content-Type", "application/json")
	}

	client := http.Client{
		Timeout: i.timeout,
	}

	res, err := client.Do(req)
	if err != nil {
		return err
	}

	if res.StatusCode != 200 {
		return fmt.Errorf("url(%s) response code: %v", i.urlPath, res.StatusCode)
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	payload, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	return json.Unmarshal(payload, i.outPtr)
}

func (i *Ibex) GET() error {
	i.Method(http.MethodGet)
	return i.do()
}

func (i *Ibex) POST() error {
	i.Method(http.MethodPost)
	return i.do()
}

func (i *Ibex) PUT() error {
	i.Method(http.MethodPut)
	return i.do()
}

func (i *Ibex) DELETE() error {
	i.Method(http.MethodDelete)
	return i.do()
}

func (i *Ibex) PATCH() error {
	i.Method(http.MethodPatch)
	return i.do()
}
