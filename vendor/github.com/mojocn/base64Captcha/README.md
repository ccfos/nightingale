# A flexible and various captcha package
[![Go Report Card](https://goreportcard.com/badge/github.com/mojocn/base64Captcha)](https://goreportcard.com/report/github.com/mojocn/base64Captcha)
[![GoDoc](https://godoc.org/github.com/mojocn/base64Captcha?status.svg)](https://godoc.org/github.com/mojocn/base64Captcha)
[![Build Status](https://travis-ci.org/mojocn/base64Captcha.svg?branch=master)](https://travis-ci.org/mojocn/base64Captcha)
[![codecov](https://codecov.io/gh/mojocn/base64Captcha/branch/master/graph/badge.svg)](https://codecov.io/gh/mojocn/base64Captcha)
![stability-stable](https://img.shields.io/badge/stability-stable-brightgreen.svg)
[![Foundation](https://img.shields.io/badge/Golang-Foundation-green.svg)](http://golangfoundation.org)

Base64captcha supports any unicode character and can easily be customized to support Math Chinese Korean Japanese Russian Arabic etc.


## 1. 📖📖📖 Doc & Demo

* [English](https://godoc.org/github.com/mojocn/base64Captcha)
* [中文文档](https://mojotv.cn/go/refactor-base64-captcha)
* [Playground](https://captcha.mojotv.cn)

## 2. 🚀🚀🚀 Quick start
### 2.1 🎬🎬🎬 Use history version
[Tag v1.2.2](https://github.com/mojocn/base64Captcha/tree/v1.2.2)

` go get github.com/mojocn/base64Captcha@v1.2.2`

or edit your `go.mod` file to

`github.com/mojocn/base64Captcha@v1.2.2`

### 2.2 📥📥📥 Download package
    go get -u github.com/mojocn/base64Captcha
For Gopher from mainland China without VPN `go get golang.org/x/image` failure solution:
- go version > 1.11
- set env `GOPROXY=https://goproxy.io`

### 2.3 🏂🏂🏂 How to code with base64Captcha

#### 2.3.1 🏇🏇🏇 Implement [Store interface](interface_store.go) or use build-in memory store

- [Build-in Memory Store](store_memory.go)

```go
type Store interface {
	// Set sets the digits for the captcha id.
	Set(id string, value string)

	// Get returns stored digits for the captcha id. Clear indicates
	// whether the captcha must be deleted from the store.
	Get(id string, clear bool) string
	
    //Verify captcha's answer directly
	Verify(id, answer string, clear bool) bool
}

```

#### 2.3.2 🏄🏄🏄 Implement [Driver interface](interface_driver.go) or use one of build-in drivers
There are some build-in drivers:
1. [Build-in Driver Digit](driver_digit.go)  
2. [Build-in Driver String](driver_string.go)
3. [Build-in Driver Math](driver_math.go)
4. [Build-in Driver Chinese](driver_chinese.go)

```go
// Driver captcha interface for captcha engine to to write staff
type Driver interface {
	//DrawCaptcha draws binary item
	DrawCaptcha(content string) (item Item, err error)
	//GenerateIdQuestionAnswer creates rand id, content and answer
	GenerateIdQuestionAnswer() (id, q, a string)
}
```

#### 2.3.3 🚴🚴🚴 ‍Core code [captcha.go](captcha.go)
`captcha.go` is the entry of base64Captcha which is quite simple.
```go
package base64Captcha

import (
	"math/rand"
	"time"
)

func init() {
	//init rand seed
	rand.Seed(time.Now().UnixNano())
}

// Captcha captcha basic information.
type Captcha struct {
	Driver Driver
	Store  Store
}

//NewCaptcha creates a captcha instance from driver and store
func NewCaptcha(driver Driver, store Store) *Captcha {
	return &Captcha{Driver: driver, Store: store}
}

//Generate generates a random id, base64 image string or an error if any
func (c *Captcha) Generate() (id, b64s string, err error) {
	id,content, answer := c.Driver.GenerateIdQuestionAnswer()
	item, err := c.Driver.DrawCaptcha(content)
	if err != nil {
		return "", "", err
	}
	c.Store.Set(id, answer)
	b64s = item.EncodeB64string()
	return
}

//Verify by a given id key and remove the captcha value in store,
//return boolean value.
//if you has multiple captcha instances which share a same store.
//You may want to call `store.Verify` method instead.
func (c *Captcha) Verify(id, answer string, clear bool) (match bool) {
	match = c.Store.Get(id, clear) == answer
	return
}

```
#### 2.3.4 🚵🚵🚵 ‍Generate Base64(image/audio) string
```go
func (c *Captcha) Generate() (id, b64s string, err error) {
	id,content, answer := c.Driver.GenerateIdQuestionAnswer()
	item, err := c.Driver.DrawCaptcha(content)
	if err != nil {
		return "", "", err
	}
	c.Store.Set(id, answer)
	b64s = item.EncodeB64string()
	return
}
```
#### 2.3.5 🤸🤸🤸 Verify Answer
```go
//if you has multiple captcha instances which shares a same store. You may want to use `store.Verify` method instead.
//Verify by given id key and remove the captcha value in store, return boolean value.
func (c *Captcha) Verify(id, answer string, clear bool) (match bool) {
	match = c.Store.Get(id, clear) == answer
	return
}
```

#### 2.3.6 🏃🏃🏃 ‍Full Example

```go
// example of HTTP server that uses the captcha package.
package main

import (
	"encoding/json"
	"fmt"
	"github.com/mojocn/base64Captcha"
	"log"
	"net/http"
)

//configJsonBody json request body.
type configJsonBody struct {
	Id            string
	CaptchaType   string
	VerifyValue   string
	DriverAudio   *base64Captcha.DriverAudio
	DriverString  *base64Captcha.DriverString
	DriverChinese *base64Captcha.DriverChinese
	DriverMath    *base64Captcha.DriverMath
	DriverDigit   *base64Captcha.DriverDigit
}

var store = base64Captcha.DefaultMemStore

// base64Captcha create http handler
func generateCaptchaHandler(w http.ResponseWriter, r *http.Request) {
	//parse request parameters
	decoder := json.NewDecoder(r.Body)
	var param configJsonBody
	err := decoder.Decode(&param)
	if err != nil {
		log.Println(err)
	}
	defer r.Body.Close()
	var driver base64Captcha.Driver

	//create base64 encoding captcha
	switch param.CaptchaType {
	case "audio":
		driver = param.DriverAudio
	case "string":
		driver = param.DriverString.ConvertFonts()
	case "math":
		driver = param.DriverMath.ConvertFonts()
	case "chinese":
		driver = param.DriverChinese.ConvertFonts()
	default:
		driver = param.DriverDigit
	}
	c := base64Captcha.NewCaptcha(driver, store)
	id, b64s, err := c.Generate()
	body := map[string]interface{}{"code": 1, "data": b64s, "captchaId": id, "msg": "success"}
	if err != nil {
		body = map[string]interface{}{"code": 0, "msg": err.Error()}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(body)
}

// base64Captcha verify http handler
func captchaVerifyHandle(w http.ResponseWriter, r *http.Request) {

	//parse request json body
	decoder := json.NewDecoder(r.Body)
	var param configJsonBody
	err := decoder.Decode(&param)
	if err != nil {
		log.Println(err)
	}
	defer r.Body.Close()
	//verify the captcha
	body := map[string]interface{}{"code": 0, "msg": "failed"}
	if store.Verify(param.Id, param.VerifyValue, true) {
		body = map[string]interface{}{"code": 1, "msg": "ok"}
	}

	//set json response
	w.Header().Set("Content-Type", "application/json; charset=utf-8")

	json.NewEncoder(w).Encode(body)
}

//start a net/http server
func main() {
	//serve Vuejs+ElementUI+Axios Web Application
	http.Handle("/", http.FileServer(http.Dir("./static")))

	//api for create captcha
	http.HandleFunc("/api/getCaptcha", generateCaptchaHandler)

	//api for verify captcha
	http.HandleFunc("/api/verifyCaptcha", captchaVerifyHandle)

	fmt.Println("Server is at :8777")
	if err := http.ListenAndServe(":8777", nil); err != nil {
		log.Fatal(err)
	}
}
```


## 3. 🎨🎨🎨 Customization
You can customize your captcha display image by implementing [interface driver](interface_driver.go) 
and [interface item](interface_item.go).

There are some example for your reference.
1. [DriverMath](driver_math.go)
2. [DriverChinese](driver_chinese.go)
3. [ItemChar](item_char.go)

***You can even design the [captcha struct](captcha.go) to whatever you prefer.***

## 4. 💖💖💖 Thanks
- [dchest/captha](https://github.com/dchest/captcha)
- [@slayercat](https://github.com/slayercat)
- [@amzyang](https://github.com/amzyang)
- [@Luckyboys](https://github.com/Luckyboys)
- [@hi-sb](https://github.com/hi-sb)

## 5. 🍭🍭🍭 Licence

base64Captcha source code is licensed under the Apache Licence, Version 2.0
(http://www.apache.org/licenses/LICENSE-2.0.html).
