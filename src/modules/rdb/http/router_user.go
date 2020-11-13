package http

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/str"

	"github.com/didi/nightingale/src/models"
)

// 通讯录，只要登录用户就可以看，超管要修改某个用户的信息，也是调用这个接口获取列表先
func userListGet(c *gin.Context) {
	limit := queryInt(c, "limit", 20)
	query := queryStr(c, "query", "")
	ids := str.IdsInt64(queryStr(c, "ids", ""))

	total, err := models.UserTotal(ids, query)
	dangerous(err)

	list, err := models.UserGets(ids, query, limit, offset(c, limit))
	dangerous(err)

	for i := 0; i < len(list); i++ {
		list[i].UUID = ""
	}

	renderData(c, gin.H{
		"list":  list,
		"total": total,
	}, nil)
}

type userProfileForm struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Dispname string `json:"dispname"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Im       string `json:"im"`
	IsRoot   int    `json:"is_root"`
	LeaderId int64  `json:"leader_id"`
}

func userAddPost(c *gin.Context) {
	root := loginRoot(c)

	var f userProfileForm
	bind(c, &f)
	dangerous(checkPassword(f.Password))

	pass, err := models.CryptoPass(f.Password)
	dangerous(err)

	u := models.User{
		Username: f.Username,
		Password: pass,
		Dispname: f.Dispname,
		Phone:    f.Phone,
		Email:    f.Email,
		Im:       f.Im,
		IsRoot:   f.IsRoot,
		LeaderId: f.LeaderId,
		UUID:     models.GenUUIDForUser(f.Username),
		CreateAt: time.Now().Unix(),
	}

	if f.LeaderId != 0 {
		u.LeaderName = User(f.LeaderId).Username
	}

	err = u.Save()
	if err == nil {
		go models.OperationLogNew(root.Username, "user", u.Id, fmt.Sprintf("UserCreate %s is_root? %v", u.Username, f.IsRoot == 1))
	}

	renderMessage(c, err)
}

func userProfileGet(c *gin.Context) {
	user := User(urlParamInt64(c, "id"))
	user.UUID = ""
	renderData(c, user, nil)
}

func userProfilePut(c *gin.Context) {
	root := loginRoot(c)

	var f userProfileForm
	bind(c, &f)

	arr := make([]string, 0, 5)

	target := User(urlParamInt64(c, "id"))

	if f.LeaderId != target.LeaderId {
		target.LeaderId = f.LeaderId
		if f.LeaderId == 0 {
			target.LeaderName = ""
		} else {
			leader := User(f.LeaderId)
			target.LeaderName = leader.Username
		}
	}

	if f.Dispname != target.Dispname {
		arr = append(arr, fmt.Sprintf("dispname: %s -> %s", target.Dispname, f.Dispname))
		target.Dispname = f.Dispname
	}

	if f.Phone != target.Phone {
		arr = append(arr, fmt.Sprintf("phone: %s -> %s", target.Phone, f.Phone))
		target.Phone = f.Phone
	}

	if f.Email != target.Email {
		arr = append(arr, fmt.Sprintf("email: %s -> %s", target.Email, f.Email))
		target.Email = f.Email
	}

	if f.Im != target.Im {
		arr = append(arr, fmt.Sprintf("im: %s -> %s", target.Im, f.Im))
		target.Im = f.Im
	}

	if f.IsRoot != target.IsRoot {
		arr = append(arr, fmt.Sprintf("is_root? %v -> %v", target.IsRoot == 1, f.IsRoot == 1))
		target.IsRoot = f.IsRoot
	}

	err := target.Update("dispname", "phone", "email", "im", "is_root", "leader_id", "leader_name")
	if err == nil && len(arr) > 0 {
		content := strings.Join(arr, "，")
		go models.OperationLogNew(root.Username, "user", target.Id, fmt.Sprintf("UserModify %s %s", target.Username, content))
	}

	renderMessage(c, err)
}

type userPasswordForm struct {
	Password string `json:"password" binding:"required"`
}

func userPasswordPut(c *gin.Context) {
	root := loginRoot(c)

	var f userPasswordForm
	bind(c, &f)
	dangerous(checkPassword(f.Password))

	target := User(urlParamInt64(c, "id"))

	pass, err := models.CryptoPass(f.Password)
	dangerous(err)

	target.Password = pass
	err = target.Update("password")
	if err == nil {
		go models.OperationLogNew(root.Username, "user", target.Id, fmt.Sprintf("UserChangePassword %s", target.Username))
	}
	renderMessage(c, err)
}

func userDel(c *gin.Context) {
	root := loginRoot(c)

	id := urlParamInt64(c, "id")
	target, err := models.UserGet("id=?", id)
	dangerous(err)

	if target == nil {
		renderMessage(c, nil)
		return
	}

	if target.Username == "root" {
		bomb("cannot delete root user")
	}

	err = target.Del()
	if err == nil {
		go models.OperationLogNew(root.Username, "user", target.Id, fmt.Sprintf("UserDelete %s", target.Username))
	}

	renderMessage(c, err)
}

func v1UsernameGetByUUID(c *gin.Context) {
	renderData(c, models.UsernameByUUID(queryStr(c, "uuid")), nil)
}

func v1UserGetByUUID(c *gin.Context) {
	user, err := models.UserGet("uuid=?", queryStr(c, "uuid"))
	dangerous(err)

	if user == nil {
		renderMessage(c, "user not found")
		return
	}

	renderData(c, user, nil)
}

func v1UserGetByUUIDs(c *gin.Context) {
	uuids := strings.Split(queryStr(c, "uuids"), ",")
	users, err := models.UserGetByUUIDs(uuids)
	renderData(c, users, err)
}

func v1UserIdsGetByTeamIds(c *gin.Context) {
	ids := queryStr(c, "ids")
	userIds, err := models.UserIdsByTeamIds(str.IdsInt64(ids))
	renderData(c, userIds, err)
}

func v1UserGetByIds(c *gin.Context) {
	ids := queryStr(c, "ids")
	users, err := models.UserGetByIds(str.IdsInt64(ids))
	renderData(c, users, err)
}

func v1UserGetByNames(c *gin.Context) {
	names := strings.Split(queryStr(c, "names"), ",")
	users, err := models.UserGetByNames(names)
	renderData(c, users, err)
}

func v1UserGetByToken(c *gin.Context) {
	ut, err := models.UserTokenGet("token=?", queryStr(c, "token"))
	dangerous(err)

	if ut == nil {
		renderMessage(c, "token not found")
		return
	}

	user, err := models.UserGet("id=?", ut.UserId)
	dangerous(err)

	if user == nil {
		renderMessage(c, "user not found")
		return
	}

	renderData(c, user, nil)
}

func userInviteGet(c *gin.Context) {
	token, err := models.CryptoPass(fmt.Sprint(time.Now().UnixNano()))
	dangerous(err)

	err = models.InviteNew(token, loginUsername(c))
	renderData(c, token, err)
}

type userInviteForm struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
	Dispname string `json:"dispname"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Im       string `json:"im"`
	Token    string `json:"token" binding:"required"`
}

func userInvitePost(c *gin.Context) {
	var f userInviteForm
	bind(c, &f)
	dangerous(checkPassword(f.Password))

	inv, err := models.InviteGet("token=?", f.Token)
	dangerous(err)

	now := time.Now().Unix()
	if inv.Expire < now {
		dangerous("invite url already expired")
	}

	u := models.User{
		Username: f.Username,
		Dispname: f.Dispname,
		Phone:    f.Phone,
		Email:    f.Email,
		Im:       f.Im,
		UUID:     models.GenUUIDForUser(f.Username),
		CreateAt: now,
	}

	u.Password, err = models.CryptoPass(f.Password)
	dangerous(err)

	renderMessage(c, u.Save())
}
