package user

import (
	openapi "github.com/alibabacloud-go/darabonba-openapi/v2/client"
	gatewayclient "github.com/alibabacloud-go/gateway-dingtalk/client"
	openapiutil "github.com/alibabacloud-go/openapi-util/service"
	util "github.com/alibabacloud-go/tea-utils/v2/service"
	"github.com/alibabacloud-go/tea/tea"
)

type GetUserQuery struct {
	AccessToken string `json:"access_token" xml:"access_token"`
}

type Client struct {
	openapi.Client
}

func NewClient(config *openapi.Config) (*Client, error) {
	client := new(Client)
	err := client.Init(config)
	return client, err
}

func (client *Client) Init(config *openapi.Config) (err error) {
	err = client.Client.Init(config)
	if err != nil {
		return err
	}
	gatewayClient, err := gatewayclient.NewClient()
	if err != nil {
		return err
	}

	client.Spi = gatewayClient
	client.EndpointRule = tea.String("")
	if tea.BoolValue(util.Empty(client.Endpoint)) {
		client.Endpoint = tea.String("oapi.dingtalk.com")
	}

	return nil
}

// Summary:
//
// 获取用户详情信息
//
// @param request - GetUserRequest
//
// @param query - GetUserQuery
//
// @return GetUserResponse
func (client *Client) GetUser(request *GetUserRequest, query *GetUserQuery) (result *GetUserResponse, err error) {
	runtime := &util.RuntimeOptions{}
	realQuery := make(map[string]*string)
	if !tea.BoolValue(util.IsUnset(query.AccessToken)) {
		realQuery["access_token"] = tea.String(query.AccessToken)
	}

	reqBody := map[string]interface{}{}
	if !tea.BoolValue(util.IsUnset(request.UserID)) {
		reqBody["userid"] = request.UserID
	}

	if !tea.BoolValue(util.IsUnset(request.Language)) {
		reqBody["language"] = request.Language
	}

	req := &openapi.OpenApiRequest{
		Query: realQuery,
		Body:  openapiutil.ParseToMap(reqBody),
	}
	params := &openapi.Params{
		Action:      tea.String("GetUser"),
		Version:     tea.String("contact_1.0"),
		Protocol:    tea.String("HTTPS"),
		Pathname:    tea.String("/topapi/v2/user/get"),
		Method:      tea.String("POST"),
		AuthType:    tea.String("AK"),
		Style:       tea.String("ROA"),
		ReqBodyType: tea.String("none"),
		BodyType:    tea.String("json"),
	}
	result = &GetUserResponse{}
	body, err := client.Execute(params, req, runtime)
	if err != nil {
		return result, err
	}
	err = tea.Convert(body, &result)
	return result, err
}

type GetUserRequest struct {
	UserID   string `json:"user_id" xml:"user_id"`
	Language string `json:"language" xml:"language"`
}

type GetUserResult struct {
	AvatarUrl *string `json:"avatarUrl,omitempty" xml:"avatarUrl,omitempty"`
	Email     *string `json:"email,omitempty" xml:"email,omitempty"`
	Mobile    *string `json:"mobile,omitempty" xml:"mobile,omitempty"`
	Name      *string `json:"name,omitempty" xml:"name,omitempty"`
	JobNumber *string `json:"job_number,omitempty" xml:"job_number,omitempty"`
	StateCode *string `json:"stateCode,omitempty" xml:"stateCode,omitempty"`
	UnionId   *string `json:"unionid,omitempty" xml:"unionid,omitempty"`
	UserId    *string `json:"userid,omitempty" xml:"userid,omitempty"`
	Visitor   *bool   `json:"visitor,omitempty" xml:"visitor,omitempty"`
}

func (s GetUserResult) String() string {
	return tea.Prettify(s)
}

func (s GetUserResult) GoString() string {
	return s.String()
}

type GetUserResponseBody struct {
	Result    *GetUserResult `json:"result,omitempty" xml:"result,omitempty"`
	RequestID *string        `json:"request_id,omitempty" xml:"request_id,omitempty"`
	ErrMsg    *string        `json:"errmsg,omitempty" xml:"errmsg,omitempty"`
	ErrCode   *int           `json:"errcode,omitempty" xml:"errcode,omitempty"`
}

func (s GetUserResponseBody) String() string {
	return tea.Prettify(s)
}

func (s GetUserResponseBody) GoString() string {
	return s.String()
}

type GetUserResponse struct {
	Headers    map[string]*string   `json:"headers,omitempty" xml:"headers,omitempty"`
	StatusCode *int32               `json:"statusCode,omitempty" xml:"statusCode,omitempty"`
	Body       *GetUserResponseBody `json:"body,omitempty" xml:"body,omitempty"`
}

func (s GetUserResponse) String() string {
	return tea.Prettify(s)
}

func (s GetUserResponse) GoString() string {
	return s.String()
}

// Summary:
//
// 根据unionid获取用户ID
//
// @param request - GetUnionIdRequest
//
// @param query - GetUserQuery
//
// @return GetUserResponse
func (client *Client) GetByUnionId(request *GetUnionIdRequest, query *GetUserQuery) (result *GetUserIDResponse, err error) {
	runtime := &util.RuntimeOptions{}
	realQuery := make(map[string]*string)
	if !tea.BoolValue(util.IsUnset(query.AccessToken)) {
		realQuery["access_token"] = tea.String(query.AccessToken)
	}

	reqBody := map[string]interface{}{}
	if !tea.BoolValue(util.IsUnset(request.UnionID)) {
		reqBody["unionid"] = request.UnionID
	}

	req := &openapi.OpenApiRequest{
		Query: realQuery,
		Body:  openapiutil.ParseToMap(reqBody),
	}
	params := &openapi.Params{
		Action:      tea.String("GetUserID"),
		Version:     tea.String("contact_1.0"),
		Protocol:    tea.String("HTTPS"),
		Pathname:    tea.String("/topapi/user/getbyunionid"),
		Method:      tea.String("POST"),
		AuthType:    tea.String("AK"),
		Style:       tea.String("ROA"),
		ReqBodyType: tea.String("none"),
		BodyType:    tea.String("json"),
	}
	result = &GetUserIDResponse{}
	body, err := client.Execute(params, req, runtime)
	if err != nil {
		return result, err
	}
	err = tea.Convert(body, &result)
	return result, err
}

type GetUnionIdRequest struct {
	UnionID string `json:"union_id" xml:"union_id"`
}

type GetUserIDResult struct {
	UserId      *string `json:"userid,omitempty" xml:"userid,omitempty"`
	ContactType *bool   `json:"contact_type,omitempty" xml:"contact_type,omitempty"`
}

func (s GetUserIDResult) String() string {
	return tea.Prettify(s)
}

func (s GetUserIDResult) GoString() string {
	return s.String()
}

type GetUserIDResponseBody struct {
	Result    *GetUserIDResult `json:"result,omitempty" xml:"result,omitempty"`
	RequestID *string          `json:"request_id,omitempty" xml:"request_id,omitempty"`
	ErrMsg    *string          `json:"errmsg,omitempty" xml:"errmsg,omitempty"`
	ErrCode   *int             `json:"errcode,omitempty" xml:"errcode,omitempty"`
}

func (s GetUserIDResponseBody) String() string {
	return tea.Prettify(s)
}

func (s GetUserIDResponseBody) GoString() string {
	return s.String()
}

type GetUserIDResponse struct {
	Headers    map[string]*string     `json:"headers,omitempty" xml:"headers,omitempty"`
	StatusCode *int32                 `json:"statusCode,omitempty" xml:"statusCode,omitempty"`
	Body       *GetUserIDResponseBody `json:"body,omitempty" xml:"body,omitempty"`
}

func (s GetUserIDResponse) String() string {
	return tea.Prettify(s)
}

func (s GetUserIDResponse) GoString() string {
	return s.String()
}
