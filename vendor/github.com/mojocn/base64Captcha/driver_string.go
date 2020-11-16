package base64Captcha

import (
	"github.com/golang/freetype/truetype"
	"image/color"
	"strings"
)

//DriverChar captcha config for captcha-engine-characters.
type DriverString struct {
	// Height png height in pixel.
	Height int

	// Width Captcha png width in pixel.
	Width int

	//NoiseCount text noise count.
	NoiseCount int

	//ShowLineOptions := OptionShowHollowLine | OptionShowSlimeLine | OptionShowSineLine .
	ShowLineOptions int

	//Length random string length.
	Length int

	//Source is a unicode which is the rand string from.
	Source string

	//BgColor captcha image background color (optional)
	BgColor *color.RGBA

	//Fonts loads by name see fonts.go's comment
	Fonts      []string
	fontsArray []*truetype.Font
}

//NewDriverString creates driver
func NewDriverString(height int, width int, noiseCount int, showLineOptions int, length int, source string, bgColor *color.RGBA, fonts []string) *DriverString {
	tfs := []*truetype.Font{}
	for _, fff := range fonts {
		tf := loadFontByName("fonts/" + fff)
		tfs = append(tfs, tf)
	}
	if len(tfs) == 0 {
		tfs = fontsAll
	}
	return &DriverString{Height: height, Width: width, NoiseCount: noiseCount, ShowLineOptions: showLineOptions, Length: length, Source: source, BgColor: bgColor, fontsArray: tfs}
}

//ConvertFonts loads fonts by names
func (d *DriverString) ConvertFonts() *DriverString {
	tfs := []*truetype.Font{}
	for _, fff := range d.Fonts {
		tf := loadFontByName("fonts/" + fff)
		tfs = append(tfs, tf)
	}
	if len(tfs) == 0 {
		tfs = fontsAll
	}
	d.fontsArray = tfs
	return d
}

//GenerateIdQuestionAnswer creates id,content and answer
func (d *DriverString) GenerateIdQuestionAnswer() (id, content, answer string) {
	id = RandomId()
	content = RandText(d.Length, d.Source)
	return id, content, content
}

//DrawCaptcha draws captcha item
func (d *DriverString) DrawCaptcha(content string) (item Item, err error) {

	var bgc color.RGBA
	if d.BgColor != nil {
		bgc = *d.BgColor
	} else {
		bgc = RandLightColor()
	}
	itemChar := NewItemChar(d.Width, d.Height, bgc)

	//draw hollow line
	if d.ShowLineOptions&OptionShowHollowLine == OptionShowHollowLine {
		itemChar.drawHollowLine()
	}

	//draw slime line
	if d.ShowLineOptions&OptionShowSlimeLine == OptionShowSlimeLine {
		itemChar.drawSlimLine(3)
	}

	//draw sine line
	if d.ShowLineOptions&OptionShowSineLine == OptionShowSineLine {
		itemChar.drawSineLine()
	}

	//draw noise
	if d.NoiseCount > 0 {
		source := TxtNumbers + TxtAlphabet + ",.[]<>"
		noise := RandText(d.NoiseCount, strings.Repeat(source, d.NoiseCount))
		err = itemChar.drawNoise(noise, d.fontsArray)
		if err != nil {
			return
		}
	}

	//draw content
	err = itemChar.drawText(content, d.fontsArray)
	if err != nil {
		return
	}

	return itemChar, nil
}
