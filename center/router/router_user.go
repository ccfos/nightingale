package router

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ccfos/nightingale/v6/models"
	"github.com/ccfos/nightingale/v6/pkg/flashduty"
	"github.com/ccfos/nightingale/v6/pkg/ormx"
	"github.com/ccfos/nightingale/v6/pkg/secu"

	"github.com/gin-gonic/gin"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
)

func (rt *Router) userBusiGroupsGets(c *gin.Context) {
	userid := ginx.QueryInt64(c, "userid", 0)
	username := ginx.QueryStr(c, "username", "")

	if userid == 0 && username == "" {
		ginx.Bomb(http.StatusBadRequest, "userid or username required")
	}

	var user *models.User
	var err error
	if userid > 0 {
		user, err = models.UserGetById(rt.Ctx, userid)
	} else {
		user, err = models.UserGetByUsername(rt.Ctx, username)
	}

	ginx.Dangerous(err)

	groups, err := user.BusiGroups(rt.Ctx, 10000, "")
	ginx.NewRender(c).Data(groups, err)
}

func (rt *Router) userFindAll(c *gin.Context) {
	list, err := models.UserGetAll(rt.Ctx)
	ginx.NewRender(c).Data(list, err)
}

func (rt *Router) userGets(c *gin.Context) {
	stime, etime := getTimeRange(c)
	limit := ginx.QueryInt(c, "limit", 20)
	query := ginx.QueryStr(c, "query", "")
	order := ginx.QueryStr(c, "order", "username")
	desc := ginx.QueryBool(c, "desc", false)
	usernames := strings.Split(ginx.QueryStr(c, "usernames", ""), ",")
	phones := strings.Split(ginx.QueryStr(c, "phones", ""), ",")
	emails := strings.Split(ginx.QueryStr(c, "emails", ""), ",")

	if len(usernames) == 1 && usernames[0] == "" {
		usernames = []string{}
	}

	if len(phones) == 1 && phones[0] == "" {
		phones = []string{}
	}

	if len(emails) == 1 && emails[0] == "" {
		emails = []string{}
	}

	go rt.UserCache.UpdateUsersLastActiveTime()
	total, err := models.UserTotal(rt.Ctx, query, stime, etime)
	ginx.Dangerous(err)

	list, err := models.UserGets(rt.Ctx, query, limit, ginx.Offset(c, limit), stime, etime, order, desc, usernames, phones, emails)
	ginx.Dangerous(err)

	user := c.MustGet("user").(*models.User)

	ginx.NewRender(c).Data(gin.H{
		"list":  list,
		"total": total,
		"admin": user.IsAdmin(),
	}, nil)
}

type userAddForm struct {
	Username string       `json:"username" binding:"required"`
	Password string       `json:"password" binding:"required"`
	Nickname string       `json:"nickname"`
	Phone    string       `json:"phone"`
	Email    string       `json:"email"`
	Portrait string       `json:"portrait"`
	Roles    []string     `json:"roles" binding:"required"`
	Contacts ormx.JSONObj `json:"contacts"`
}

func (rt *Router) userAddPost(c *gin.Context) {
	var f userAddForm
	ginx.BindJSON(c, &f)

	authPassWord := f.Password
	if rt.HTTP.RSA.OpenRSA {
		decPassWord, err := secu.Decrypt(f.Password, rt.HTTP.RSA.RSAPrivateKey, rt.HTTP.RSA.RSAPassWord)
		if err != nil {
			logger.Errorf("RSA Decrypt failed: %v username: %s", err, f.Username)
			ginx.NewRender(c).Message(err)
			return
		}
		authPassWord = decPassWord
	}

	password, err := models.CryptoPass(rt.Ctx, authPassWord)
	ginx.Dangerous(err)

	if len(f.Roles) == 0 {
		ginx.Bomb(http.StatusBadRequest, "roles empty")
	}

	username := Username(c)

	u := models.User{
		Username: f.Username,
		Password: password,
		Nickname: f.Nickname,
		Phone:    f.Phone,
		Email:    f.Email,
		Portrait: f.Portrait,
		Roles:    strings.Join(f.Roles, " "),
		Contacts: f.Contacts,
		CreateBy: username,
		UpdateBy: username,
	}

	ginx.Dangerous(u.Verify())
	ginx.NewRender(c).Message(u.Add(rt.Ctx))
}

func (rt *Router) userProfileGet(c *gin.Context) {
	user := User(rt.Ctx, ginx.UrlParamInt64(c, "id"))
	ginx.NewRender(c).Data(user, nil)
}

type userProfileForm struct {
	Nickname string       `json:"nickname"`
	Phone    string       `json:"phone"`
	Email    string       `json:"email"`
	Roles    []string     `json:"roles"`
	Contacts ormx.JSONObj `json:"contacts"`
}

func (rt *Router) userProfilePutByService(c *gin.Context) {
	var f models.User
	ginx.BindJSON(c, &f)

	if len(f.RolesLst) == 0 {
		ginx.Bomb(http.StatusBadRequest, "roles empty")
	}

	password, err := models.CryptoPass(rt.Ctx, f.Password)
	ginx.Dangerous(err)

	target := User(rt.Ctx, ginx.UrlParamInt64(c, "id"))
	target.Nickname = f.Nickname
	target.Password = password
	target.Phone = f.Phone
	target.Email = f.Email
	target.Portrait = f.Portrait
	target.Roles = strings.Join(f.RolesLst, " ")
	target.Contacts = f.Contacts
	target.UpdateBy = Username(c)

	ginx.NewRender(c).Message(target.UpdateAllFields(rt.Ctx))
}

func (rt *Router) userProfilePut(c *gin.Context) {
	var f userProfileForm
	ginx.BindJSON(c, &f)

	if len(f.Roles) == 0 {
		ginx.Bomb(http.StatusBadRequest, "roles empty")
	}

	target := User(rt.Ctx, ginx.UrlParamInt64(c, "id"))
	oldInfo := models.User{
		Username: target.Username,
		Phone:    target.Phone,
		Email:    target.Email,
	}
	target.Nickname = f.Nickname
	target.Phone = f.Phone
	target.Email = f.Email
	target.Roles = strings.Join(f.Roles, " ")
	target.Contacts = f.Contacts
	target.UpdateBy = c.MustGet("username").(string)

	if flashduty.NeedSyncUser(rt.Ctx) {
		flashduty.UpdateUser(rt.Ctx, oldInfo, f.Email, f.Phone)
	}

	ginx.NewRender(c).Message(target.UpdateAllFields(rt.Ctx))
}

type userPasswordForm struct {
	Password string `json:"password" binding:"required"`
}

func (rt *Router) userPasswordPut(c *gin.Context) {
	var f userPasswordForm
	ginx.BindJSON(c, &f)

	target := User(rt.Ctx, ginx.UrlParamInt64(c, "id"))

	authPassWord := f.Password
	if rt.HTTP.RSA.OpenRSA {
		decPassWord, err := secu.Decrypt(f.Password, rt.HTTP.RSA.RSAPrivateKey, rt.HTTP.RSA.RSAPassWord)
		if err != nil {
			logger.Errorf("RSA Decrypt failed: %v username: %s", err, target.Username)
			ginx.NewRender(c).Message(err)
			return
		}
		authPassWord = decPassWord
	}

	cryptoPass, err := models.CryptoPass(rt.Ctx, authPassWord)
	ginx.Dangerous(err)

	ginx.NewRender(c).Message(target.UpdatePassword(rt.Ctx, cryptoPass, c.MustGet("username").(string)))
}

func (rt *Router) userDel(c *gin.Context) {
	id := ginx.UrlParamInt64(c, "id")
	target, err := models.UserGetById(rt.Ctx, id)
	ginx.Dangerous(err)

	if target == nil {
		ginx.NewRender(c).Message(nil)
		return
	}

	ginx.NewRender(c).Message(target.Del(rt.Ctx))
}

func (rt *Router) installDateGet(c *gin.Context) {
	rootUser, err := models.UserGetByUsername(rt.Ctx, "root")
	if err != nil {
		logger.Errorf("get root user failed: %v", err)
		ginx.NewRender(c).Data(0, nil)
		return
	}

	if rootUser == nil {
		logger.Errorf("root user not found")
		ginx.NewRender(c).Data(0, nil)
		return
	}

	ginx.NewRender(c).Data(rootUser.CreateAt, nil)
}

// usersPhoneEncrypt 统一手机号加密
func (rt *Router) usersPhoneEncrypt(c *gin.Context) {
	// 先启用手机号加密功能
	err := models.SetPhoneEncryptionEnabled(rt.Ctx, true)
	if err != nil {
		ginx.NewRender(c).Message(fmt.Errorf("开启手机号加密功能失败: %v", err))
		return
	}

	// 获取所有用户
	users, err := models.UserGetAll(rt.Ctx)
	if err != nil {
		ginx.NewRender(c).Message(fmt.Errorf("获取用户列表失败: %v", err))
		return
	}

	// 获取RSA密钥
	_, publicKey, _, err := models.GetRSAKeys(rt.Ctx)
	if err != nil {
		ginx.NewRender(c).Message(fmt.Errorf("获取RSA密钥失败: %v", err))
		return
	}

	successCount := 0
	failCount := 0

	// 对每个用户的手机号进行加密
	for _, user := range users {
		if user.Phone == "" {
			continue
		}

		// 检查是否已经加密（简单检查是否包含Base64字符）
		if isPhoneEncrypted(user.Phone) {
			continue
		}

		// 对手机号进行加密
		encryptedPhone, err := secu.EncryptValue(user.Phone, publicKey)
		if err != nil {
			logger.Errorf("Failed to encrypt phone for user %s: %v", user.Username, err)
			failCount++
			continue
		}

		// 直接更新数据库中的手机号字段（绕过GORM钩子）
		err = models.DB(rt.Ctx).Model(&models.User{}).Where("id = ?", user.Id).Update("phone", encryptedPhone).Error
		if err != nil {
			logger.Errorf("Failed to update phone for user %s: %v", user.Username, err)
			failCount++
			continue
		}

		successCount++
		logger.Debugf("Successfully encrypted phone for user %s", user.Username)
	}

	ginx.NewRender(c).Data(gin.H{
		"message":       "手机号加密操作完成",
		"success_count": successCount,
		"fail_count":    failCount,
	}, nil)
}

// usersPhoneDecrypt 统一手机号解密
func (rt *Router) usersPhoneDecrypt(c *gin.Context) {
	// 先关闭手机号加密功能
	err := models.SetPhoneEncryptionEnabled(rt.Ctx, false)
	if err != nil {
		ginx.NewRender(c).Message(fmt.Errorf("关闭手机号加密功能失败: %v", err))
		return
	}

	// 获取所有用户
	users, err := models.UserGetAll(rt.Ctx)
	if err != nil {
		ginx.NewRender(c).Message(fmt.Errorf("获取用户列表失败: %v", err))
		return
	}

	// 获取RSA密钥
	privateKey, _, password, err := models.GetRSAKeys(rt.Ctx)
	if err != nil {
		ginx.NewRender(c).Message(fmt.Errorf("获取RSA密钥失败: %v", err))
		return
	}

	successCount := 0
	failCount := 0

	// 对每个用户的手机号进行解密
	for _, user := range users {
		if user.Phone == "" {
			continue
		}

		// 检查是否是加密的手机号
		if !isPhoneEncrypted(user.Phone) {
			continue
		}

		// 对手机号进行解密
		decryptedPhone, err := secu.Decrypt(user.Phone, privateKey, password)
		if err != nil {
			logger.Errorf("Failed to decrypt phone for user %s: %v", user.Username, err)
			failCount++
			continue
		}

		// 直接更新数据库中的手机号字段（绕过GORM钩子）
		err = models.DB(rt.Ctx).Model(&models.User{}).Where("id = ?", user.Id).Update("phone", decryptedPhone).Error
		if err != nil {
			logger.Errorf("Failed to update phone for user %s: %v", user.Username, err)
			failCount++
			continue
		}

		successCount++
		logger.Debugf("Successfully decrypted phone for user %s", user.Username)
	}

	ginx.NewRender(c).Data(gin.H{
		"message":       "手机号解密操作完成",
		"success_count": successCount,
		"fail_count":    failCount,
	}, nil)
}

// isPhoneEncrypted 简单检查手机号是否已经加密
func isPhoneEncrypted(phone string) bool {
	// 如果包含Base64字符且长度较长，可能是已加密的数据
	if len(phone) > 20 && (len(phone)%4 == 0) {
		for _, c := range phone {
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/' || c == '=' {
				continue
			}
			return false
		}
		return true
	}
	return false
}
