package base64Captcha

// Driver captcha interface for captcha engine to to write staff
type Driver interface {
	//DrawCaptcha draws binary item
	DrawCaptcha(content string) (item Item, err error)
	//GenerateIdQuestionAnswer creates rand id, content and answer
	GenerateIdQuestionAnswer() (id, q, a string)
}
