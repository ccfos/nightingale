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
	"gorm.io/gorm"
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

	// 如果要删除的用户是 admin 角色，检查是否是最后一个 admin
	if target.IsAdmin() {
		adminCount, err := models.CountAdminUsers(rt.Ctx)
		ginx.Dangerous(err)

		if adminCount <= 1 {
			ginx.Bomb(http.StatusBadRequest, "Cannot delete the last admin user")
		}
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
	users, err := models.UserGetAll(rt.Ctx)
	if err != nil {
		ginx.NewRender(c).Message(fmt.Errorf("get users failed: %v", err))
		return
	}

	// 获取RSA密钥
	_, publicKey, _, err := models.GetRSAKeys(rt.Ctx)
	if err != nil {
		ginx.NewRender(c).Message(fmt.Errorf("get RSA keys failed: %v", err))
		return
	}

	// 先启用手机号加密功能
	err = models.SetPhoneEncryptionEnabled(rt.Ctx, true)
	if err != nil {
		ginx.NewRender(c).Message(fmt.Errorf("enable phone encryption failed: %v", err))
		return
	}

	// 刷新配置缓存
	err = models.RefreshPhoneEncryptionCache(rt.Ctx)
	if err != nil {
		logger.Errorf("Failed to refresh phone encryption cache: %v", err)
		// 回滚配置
		models.SetPhoneEncryptionEnabled(rt.Ctx, false)
		ginx.NewRender(c).Message(fmt.Errorf("refresh cache failed: %v", err))
		return
	}

	successCount := 0
	failCount := 0
	var failedUsers []string

	// 使用事务处理所有用户的手机号加密
	err = models.DB(rt.Ctx).Transaction(func(tx *gorm.DB) error {
		// 对每个用户的手机号进行加密
		for _, user := range users {
			if user.Phone == "" {
				continue
			}

			if isPhoneEncrypted(user.Phone) {
				continue
			}

			encryptedPhone, err := secu.EncryptValue(user.Phone, publicKey)
			if err != nil {
				logger.Errorf("Failed to encrypt phone for user %s: %v", user.Username, err)
				failCount++
				failedUsers = append(failedUsers, user.Username)
				continue
			}

			err = tx.Model(&models.User{}).Where("id = ?", user.Id).Update("phone", encryptedPhone).Error
			if err != nil {
				logger.Errorf("Failed to update phone for user %s: %v", user.Username, err)
				failCount++
				failedUsers = append(failedUsers, user.Username)
				continue
			}

			successCount++
			logger.Debugf("Successfully encrypted phone for user %s", user.Username)
		}

		// 如果有失败的用户，回滚事务
		if failCount > 0 {
			return fmt.Errorf("encrypt failed users: %d, failed users: %v", failCount, failedUsers)
		}

		return nil
	})

	if err != nil {
		// 加密失败，回滚配置
		models.SetPhoneEncryptionEnabled(rt.Ctx, false)
		models.RefreshPhoneEncryptionCache(rt.Ctx)
		ginx.NewRender(c).Message(fmt.Errorf("encrypt phone failed: %v", err))
		return
	}

	ginx.NewRender(c).Data(gin.H{
		"success_count": successCount,
		"fail_count":    failCount,
	}, nil)
}

// usersPhoneDecrypt 统一手机号解密
func (rt *Router) usersPhoneDecrypt(c *gin.Context) {
	// 先关闭手机号加密功能
	err := models.SetPhoneEncryptionEnabled(rt.Ctx, false)
	if err != nil {
		ginx.NewRender(c).Message(fmt.Errorf("disable phone encryption failed: %v", err))
		return
	}

	// 刷新配置缓存
	err = models.RefreshPhoneEncryptionCache(rt.Ctx)
	if err != nil {
		logger.Errorf("Failed to refresh phone encryption cache: %v", err)
		// 回滚配置
		models.SetPhoneEncryptionEnabled(rt.Ctx, true)
		ginx.NewRender(c).Message(fmt.Errorf("refresh cache failed: %v", err))
		return
	}

	// 获取所有用户（此时加密开关已关闭，直接读取数据库原始数据）
	var users []*models.User
	err = models.DB(rt.Ctx).Find(&users).Error
	if err != nil {
		// 回滚配置
		models.SetPhoneEncryptionEnabled(rt.Ctx, true)
		models.RefreshPhoneEncryptionCache(rt.Ctx)
		ginx.NewRender(c).Message(fmt.Errorf("get users failed: %v", err))
		return
	}

	// 获取RSA密钥
	privateKey, _, password, err := models.GetRSAKeys(rt.Ctx)
	if err != nil {
		// 回滚配置
		models.SetPhoneEncryptionEnabled(rt.Ctx, true)
		models.RefreshPhoneEncryptionCache(rt.Ctx)
		ginx.NewRender(c).Message(fmt.Errorf("get RSA keys failed: %v", err))
		return
	}

	successCount := 0
	failCount := 0
	var failedUsers []string

	// 使用事务处理所有用户的手机号解密
	err = models.DB(rt.Ctx).Transaction(func(tx *gorm.DB) error {
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
				failedUsers = append(failedUsers, user.Username)
				continue
			}

			// 直接更新数据库中的手机号字段（绕过GORM钩子）
			err = tx.Model(&models.User{}).Where("id = ?", user.Id).Update("phone", decryptedPhone).Error
			if err != nil {
				logger.Errorf("Failed to update phone for user %s: %v", user.Username, err)
				failCount++
				failedUsers = append(failedUsers, user.Username)
				continue
			}

			successCount++
			logger.Debugf("Successfully decrypted phone for user %s", user.Username)
		}

		// 如果有失败的用户，回滚事务
		if failCount > 0 {
			return fmt.Errorf("decrypt failed users: %d, failed users: %v", failCount, failedUsers)
		}

		return nil
	})

	if err != nil {
		// 解密失败，回滚配置
		models.SetPhoneEncryptionEnabled(rt.Ctx, true)
		models.RefreshPhoneEncryptionCache(rt.Ctx)
		ginx.NewRender(c).Message(fmt.Errorf("decrypt phone failed: %v", err))
		return
	}

	ginx.NewRender(c).Data(gin.H{
		"success_count": successCount,
		"fail_count":    failCount,
	}, nil)
}

// isPhoneEncrypted 检查手机号是否已经加密
func isPhoneEncrypted(phone string) bool {
	// 检查是否有 "enc:" 前缀标记
	return len(phone) > 4 && phone[:4] == "enc:"
}
