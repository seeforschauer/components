package login

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	textTemplate "text/template"
	"time"

	"github.com/GoAdminGroup/components/login/theme1"
	"github.com/GoAdminGroup/go-admin/modules/logger"
	"github.com/GoAdminGroup/go-admin/modules/utils"
	captcha2 "github.com/GoAdminGroup/go-admin/plugins/admin/modules/captcha"
	template2 "github.com/GoAdminGroup/go-admin/template"
	"github.com/GoAdminGroup/go-admin/template/login"
	"github.com/dchest/captcha"
)

var themes = make(map[string]Theme)

func init() {
	Register("theme1", new(theme1.Theme1))
}

func Register(key string, theme Theme) {
	if _, ok := themes[key]; ok {
		panic("duplicate login theme")
	}
	themes[key] = theme
}

type Login struct {
	TencentWaterProofWallData TencentWaterProofWallData
	CaptchaDigits             int
	CaptchaID                 string
	CaptchaImgSrc             string
	Theme                     string
}

type TencentWaterProofWallData struct {
	ID           string
	AppID        string
	AppSecretKey string
}

type Config struct {
	TencentWaterProofWallData TencentWaterProofWallData
	CaptchaDigits             int
	Theme                     string
}

const (
	CaptchaDriverKeyTencent = "tencent"
	CaptchaDriverKeyDefault = "digits"

	CaptchaDisableDuration = time.Minute * 2
)

type CaptchaDataItem struct {
	Time time.Time
	Data string
	Num  int
}

type CaptchaData map[string]CaptchaDataItem

func (c *CaptchaData) Clean() {
	for key, value := range *c {
		if value.Time.Add(CaptchaDisableDuration).Before(time.Now()) {
			delete(*c, key)
		}
	}
}

var captchaData = make(CaptchaData)

type DigitsCaptcha struct{}

func (c *DigitsCaptcha) Validate(token string) bool {
	tokenArr := strings.Split(token, ",")
	if len(tokenArr) < 2 {
		return false
	}
	if v, ok := captchaData[tokenArr[1]]; ok {
		if v.Data == tokenArr[0] {
			delete(captchaData, tokenArr[1])
			return true
		} else {
			v.Num++
			captchaData[tokenArr[1]] = v
			return false
		}
	}
	return false
}

type TencentCaptcha struct {
	Aid       string
	AppSecret string
}

type TencentCaptchaRes struct {
	Response  string `json:"response"`
	EvilLevel string `json:"evil_level"`
	ErrMsg    string `json:"err_msg"`
}

func (c *TencentCaptcha) Validate(token string) bool {

	u := "https://ssl.captcha.qq.com/ticket/verify?"

	v := url.Values{
		"aid":          {c.Aid},
		"AppSecretKey": {c.AppSecret},
		"Ticket":       {token},
		"Randstr":      {utils.Uuid(10)},
		"UserIP":       {"127.0.0.1"},
	}

	req, err := http.NewRequest("GET", u+v.Encode(), nil)
	if err != nil {
		return false
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}

	defer func() {
		_ = res.Body.Close()
	}()
	body, err := ioutil.ReadAll(res.Body)

	if err != nil {
		return false
	}

	var captchaRes TencentCaptchaRes
	err = json.Unmarshal(body, &captchaRes)

	if err != nil {
		return false
	}

	return captchaRes.Response == "1"
}

func Init(cfg ...Config) {
	template2.AddLoginComp(Get(cfg...))
}

func Get(cfg ...Config) *Login {
	if len(cfg) > 0 {

		if cfg[0].CaptchaDigits != 0 && cfg[0].TencentWaterProofWallData.ID == "" {
			captchaData.Clean()
			captcha2.Add(CaptchaDriverKeyDefault, new(DigitsCaptcha))
		}

		if cfg[0].TencentWaterProofWallData.ID != "" {
			captcha2.Add(CaptchaDriverKeyTencent, new(TencentCaptcha))
		}

		if cfg[0].Theme == "" {
			cfg[0].Theme = "theme1"
		}

		return &Login{
			TencentWaterProofWallData: cfg[0].TencentWaterProofWallData,
			CaptchaDigits:             cfg[0].CaptchaDigits,
			Theme:                     cfg[0].Theme,
		}
	}
	return &Login{Theme: "theme1"}
}

func byteToStr(b []byte) string {
	s := ""
	for i := 0; i < len(b); i++ {
		s += fmt.Sprintf("%v", b[i])
	}
	return s
}

func (l *Login) GetTemplate() (*template.Template, string) {

	if l.CaptchaDigits != 0 {
		id := utils.Uuid(10)
		digitByte := captcha.RandomDigits(l.CaptchaDigits)
		captchaData[id] = CaptchaDataItem{
			Data: byteToStr(digitByte),
			Time: time.Now(),
			Num:  0,
		}
		img := captcha.NewImage(id, digitByte, 110, 34)
		buf := new(bytes.Buffer)
		_, _ = img.WriteTo(buf)
		l.CaptchaID = id
		l.CaptchaImgSrc = "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	}

	t := textTemplate.New("login_theme1").Delims("{%", "%}")
	t, err := t.Parse(themes[l.Theme].GetHTML())
	if err != nil {
		logger.Error("login component, get template parse error: ", err)
	}
	buf := new(bytes.Buffer)
	err = t.Execute(buf, l)
	if err != nil {
		logger.Error("login component, get template execute error: ", err)
	}

	tmpl, err := template.New("login_theme1").
		Funcs(login.DefaultFuncMap).
		Parse(buf.String())

	if err != nil {
		logger.Error("login component, get template error: ", err)
	}

	return tmpl, "login_theme1"
}

func (l *Login) GetAssetList() []string               { return themes[l.Theme].GetAssetList() }
func (l *Login) GetAsset(name string) ([]byte, error) { return themes[l.Theme].GetAsset(name[1:]) }
func (l *Login) GetName() string                      { return "login" }
func (l *Login) IsAPage() bool                        { return true }

func (l *Login) GetContent() template.HTML {
	buffer := new(bytes.Buffer)
	tmpl, defineName := l.GetTemplate()
	err := tmpl.ExecuteTemplate(buffer, defineName, l)
	if err != nil {
		logger.Error("login component, compose html error:", err)
	}
	return template.HTML(buffer.String())
}

type Theme interface {
	GetAssetList() []string
	GetAsset(name string) ([]byte, error)
	GetHTML() string
}
