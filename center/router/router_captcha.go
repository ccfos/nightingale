package router

import (
	"context"
	"time"

	"github.com/ccfos/nightingale/v6/storage"
	"github.com/gin-gonic/gin"
	captcha "github.com/mojocn/base64Captcha"
	"github.com/toolkits/pkg/ginx"
	"github.com/toolkits/pkg/logger"
)

type CaptchaRedisStore struct {
	redis storage.Redis
}

func (s *CaptchaRedisStore) Set(id string, value string) error {
	ctx := context.Background()
	err := s.redis.Set(ctx, id, value, time.Duration(300*time.Second)).Err()
	if err != nil {
		logger.Errorf("captcha id set to redis error : %s", err.Error())
		return err
	}
	return nil
}

func (s *CaptchaRedisStore) Get(id string, clear bool) string {
	ctx := context.Background()
	val, err := s.redis.Get(ctx, id).Result()
	if err != nil {
		logger.Errorf("captcha id get from redis error : %s", err.Error())
		return ""
	}

	if clear {
		s.redis.Del(ctx, id)
	}

	return val
}

func (s *CaptchaRedisStore) Verify(id, answer string, clear bool) bool {

	old := s.Get(id, clear)
	return old == answer
}

func (rt *Router) newCaptchaRedisStore() *CaptchaRedisStore {
	if captchaStore == nil {
		captchaStore = &CaptchaRedisStore{redis: rt.Redis}
	}
	return captchaStore
}

var captchaStore *CaptchaRedisStore

type CaptchaReqBody struct {
	Id          string
	VerifyValue string
}

// 生成图形验证码
func (rt *Router) generateCaptcha(c *gin.Context) {
	var driver = captcha.NewDriverMath(60, 200, 0, captcha.OptionShowHollowLine, nil, nil, []string{"wqy-microhei.ttc"})
	cc := captcha.NewCaptcha(driver, rt.newCaptchaRedisStore())
	//data:image/png;base64
	id, b64s, err := cc.Generate()

	if err != nil {
		ginx.NewRender(c).Message(err)
		return
	}

	ginx.NewRender(c).Data(gin.H{
		"imgdata":   b64s,
		"captchaid": id,
	}, nil)
}

// 验证
func (rt *Router) captchaVerify(c *gin.Context) {

	var param CaptchaReqBody
	ginx.BindJSON(c, &param)

	//verify the captcha
	if captchaStore.Verify(param.Id, param.VerifyValue, true) {
		ginx.NewRender(c).Message("")
		return
	}
	ginx.NewRender(c).Message("incorrect verification code")
}

// 验证码开关
func (rt *Router) ifShowCaptcha(c *gin.Context) {

	if rt.HTTP.ShowCaptcha.Enable {
		ginx.NewRender(c).Data(gin.H{
			"show": true,
		}, nil)
		return
	}

	ginx.NewRender(c).Data(gin.H{
		"show": false,
	}, nil)
}

// 验证
func CaptchaVerify(id string, value string) bool {
	//verify the captcha
	return captchaStore.Verify(id, value, true)
}
