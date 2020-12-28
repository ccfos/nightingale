package base64Captcha

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/golang/freetype"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"log"
	"math"
	"math/rand"
)

//ItemChar captcha item of unicode characters
type ItemChar struct {
	bgColor color.Color
	width   int
	height  int
	nrgba   *image.NRGBA
}

//NewItemChar creates a captcha item of characters
func NewItemChar(w int, h int, bgColor color.RGBA) *ItemChar {
	d := ItemChar{width: w, height: h}
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(m, m.Bounds(), &image.Uniform{bgColor}, image.ZP, draw.Src)
	d.nrgba = m
	return &d
}

//drawHollowLine draw strong and bold white line.
func (item *ItemChar) drawHollowLine() *ItemChar {

	first := item.width / 20
	end := first * 19

	lineColor := RandLightColor()

	x1 := float64(rand.Intn(first))
	//y1 := float64(rand.Intn(y)+y);

	x2 := float64(rand.Intn(first) + end)

	multiple := float64(rand.Intn(5)+3) / float64(5)
	if int(multiple*10)%3 == 0 {
		multiple = multiple * -1.0
	}

	w := item.height / 20

	for ; x1 < x2; x1++ {

		y := math.Sin(x1*math.Pi*multiple/float64(item.width)) * float64(item.height/3)

		if multiple < 0 {
			y = y + float64(item.height/2)
		}
		item.nrgba.Set(int(x1), int(y), lineColor)

		for i := 0; i <= w; i++ {
			item.nrgba.Set(int(x1), int(y)+i, lineColor)
		}
	}

	return item
}

//drawSineLine draw a sine line.
func (item *ItemChar) drawSineLine() *ItemChar {
	var py float64

	//振幅
	a := rand.Intn(item.height / 2)

	//Y轴方向偏移量
	b := random(int64(-item.height/4), int64(item.height/4))

	//X轴方向偏移量
	f := random(int64(-item.height/4), int64(item.height/4))
	// 周期
	var t float64
	if item.height > item.width/2 {
		t = random(int64(item.width/2), int64(item.height))
	} else if item.height == item.width/2 {
		t = float64(item.height)
	} else {
		t = random(int64(item.height), int64(item.width/2))
	}
	w := float64((2 * math.Pi) / t)

	// 曲线横坐标起始位置
	px1 := 0
	px2 := int(random(int64(float64(item.width)*0.8), int64(item.width)))

	c := RandDeepColor()

	for px := px1; px < px2; px++ {
		if w != 0 {
			py = float64(a)*math.Sin(w*float64(px)+f) + b + (float64(item.width) / float64(5))
			i := item.height / 5
			for i > 0 {
				item.nrgba.Set(px+i, int(py), c)
				//fmt.Println(px + i,int(py) )
				i--
			}
		}
	}

	return item
}

//drawSlimLine draw n slim-random-color lines.
func (item *ItemChar) drawSlimLine(num int) *ItemChar {

	first := item.width / 10
	end := first * 9

	y := item.height / 3

	for i := 0; i < num; i++ {

		point1 := point{X: rand.Intn(first), Y: rand.Intn(y)}
		point2 := point{X: rand.Intn(first) + end, Y: rand.Intn(y)}

		if i%2 == 0 {
			point1.Y = rand.Intn(y) + y*2
			point2.Y = rand.Intn(y)
		} else {
			point1.Y = rand.Intn(y) + y*(i%2)
			point2.Y = rand.Intn(y) + y*2
		}

		item.drawBeeline(point1, point2, RandDeepColor())

	}
	return item
}

func (item *ItemChar) drawBeeline(point1 point, point2 point, lineColor color.RGBA) {
	dx := math.Abs(float64(point1.X - point2.X))

	dy := math.Abs(float64(point2.Y - point1.Y))
	sx, sy := 1, 1
	if point1.X >= point2.X {
		sx = -1
	}
	if point1.Y >= point2.Y {
		sy = -1
	}
	err := dx - dy
	for {
		item.nrgba.Set(point1.X, point1.Y, lineColor)
		item.nrgba.Set(point1.X+1, point1.Y, lineColor)
		item.nrgba.Set(point1.X-1, point1.Y, lineColor)
		item.nrgba.Set(point1.X+2, point1.Y, lineColor)
		item.nrgba.Set(point1.X-2, point1.Y, lineColor)
		if point1.X == point2.X && point1.Y == point2.Y {
			return
		}
		e2 := err * 2
		if e2 > -dy {
			err -= dy
			point1.X += sx
		}
		if e2 < dx {
			err += dx
			point1.Y += sy
		}
	}
}

func (item *ItemChar) drawNoise(noiseText string, fonts []*truetype.Font) error {

	c := freetype.NewContext()
	c.SetDPI(imageStringDpi)

	c.SetClip(item.nrgba.Bounds())
	c.SetDst(item.nrgba)
	c.SetHinting(font.HintingFull)
	rawFontSize := float64(item.height) / (1 + float64(rand.Intn(7))/float64(10))

	for _, char := range noiseText {
		rw := rand.Intn(item.width)
		rh := rand.Intn(item.height)
		fontSize := rawFontSize/2 + float64(rand.Intn(5))
		c.SetSrc(image.NewUniform(RandLightColor()))
		c.SetFontSize(fontSize)
		c.SetFont(randFontFrom(fonts))
		pt := freetype.Pt(rw, rh)
		if _, err := c.DrawString(string(char), pt); err != nil {
			log.Println(err)
		}
	}
	return nil
}

//drawText draw captcha string to image.把文字写入图像验证码

func (item *ItemChar) drawText(text string, fonts []*truetype.Font) error {
	c := freetype.NewContext()
	c.SetDPI(imageStringDpi)
	c.SetClip(item.nrgba.Bounds())
	c.SetDst(item.nrgba)
	c.SetHinting(font.HintingFull)

	if len(text) == 0 {
		return errors.New("text must not be empty, there is nothing to draw")
	}

	fontWidth := item.width / len(text)

	for i, s := range text {
		fontSize := float64(item.height) / (1 + float64(rand.Intn(7))/float64(9))
		c.SetSrc(image.NewUniform(RandDeepColor()))
		c.SetFontSize(fontSize)
		//useFont := randFontFrom(fonts)
		c.SetFont(randFontFrom(fonts))
		x := int(fontWidth)*i + int(fontWidth)/int(fontSize)
		//todo y 坐标要修复
		y := 5 + rand.Intn(item.height/2) + int(fontSize/2)
		pt := freetype.Pt(x, y)
		if _, err := c.DrawString(string(s), pt); err != nil {
			return err
		}
	}
	return nil
}

//BinaryEncoding encodes an image to PNG and returns a byte slice.
func (item *ItemChar) BinaryEncoding() []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, item.nrgba); err != nil {
		panic(err.Error())
	}
	return buf.Bytes()
}

// WriteTo writes captcha character in png format into the given io.Writer, and
// returns the number of bytes written and an error if any.
func (item *ItemChar) WriteTo(w io.Writer) (int64, error) {
	n, err := w.Write(item.BinaryEncoding())
	return int64(n), err
}

// EncodeB64string encodes an image to base64 string
func (item *ItemChar) EncodeB64string() string {
	return fmt.Sprintf("data:%s;base64,%s", MimeTypeImage, base64.StdEncoding.EncodeToString(item.BinaryEncoding()))
}

type point struct {
	X int
	Y int
}
