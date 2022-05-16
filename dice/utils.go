package dice

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/fy0/lockfree"
	"math/rand"
	"net"
	"regexp"
	"strings"
	"time"
)

type Int64SliceDesc []int64

func (x Int64SliceDesc) Len() int           { return len(x) }
func (x Int64SliceDesc) Less(i, j int) bool { return x[i] > x[j] }
func (x Int64SliceDesc) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }

func RemoveSpace(s string) string {
	re := regexp.MustCompile(`\s+`)
	return re.ReplaceAllString(s, "")
}

const letterBytes = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ123456789"
const letterBytes2 = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKMNOPQRSTUVWXYZ1234567890!@#$%^&*()_+=-"

const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

var randSrc = rand.NewSource(time.Now().UnixNano())

func RandStringBytesMaskImprSrcSB(n int) string {
	sb := strings.Builder{}
	sb.Grow(n)
	// A randSrc.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, randSrc.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = randSrc.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes) {
			sb.WriteByte(letterBytes[idx])
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return sb.String()
}

func RandStringBytesMaskImprSrcSB2(n int) string {
	sb := strings.Builder{}
	sb.Grow(n)
	// A randSrc.Int63() generates 63 random bits, enough for letterIdxMax characters!
	for i, cache, remain := n-1, randSrc.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = randSrc.Int63(), letterIdxMax
		}
		if idx := int(cache & letterIdxMask); idx < len(letterBytes2) {
			sb.WriteByte(letterBytes2[idx])
			i--
		}
		cache >>= letterIdxBits
		remain--
	}

	return sb.String()
}

func JsonNumberUnmarshal(data []byte, v interface{}) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	return d.Decode(v)
}

func VMValueConvert(val *VMValue, v *map[string]*VMValue, key string) *VMValue {
	if val.TypeId == VMTypeInt64 {
		n, ok := val.Value.(json.Number)
		if !ok {
			return nil
		}
		if i, err := n.Int64(); err == nil {
			if v != nil {
				(*v)[key] = &VMValue{VMTypeInt64, i}
			}
			return &VMValue{VMTypeInt64, i}
		}
	}
	if val.TypeId == -1 {
		// 先攻列表
		m2 := map[string]int64{}

		m, ok := val.Value.(map[string]interface{})
		if ok {
			for k, v := range m {
				n, ok := v.(json.Number)
				if !ok {
					continue
				}
				if i, err := n.Int64(); err == nil {
					m2[k] = i
					continue
				}
			}
			for k, v := range m {
				n, ok := v.(float64)
				if !ok {
					continue
				}
				m2[k] = int64(n)
			}
		} else {
			m2, _ = val.Value.(map[string]int64)
		}
		if v != nil {
			(*v)[key] = &VMValue{-1, m2}
		}
		return &VMValue{-1, m2}
	}
	if val.TypeId == VMTypeComputedValue {
		tmp := val.Value.(map[string]interface{})
		baseValue := tmp["base_value"].(map[string]interface{})

		val, _ := baseValue["typeId"].(json.Number).Int64()
		bv := VMValueConvert(&VMValue{
			VMValueType(val),
			baseValue["value"],
		}, nil, "")

		vd := &VMComputedValueData{
			BaseValue: *bv,
			Expr:      tmp["expr"].(string),
		}

		if v != nil {
			(*v)[key] = &VMValue{VMTypeComputedValue, vd}
		}
		//val.Value.(map)
	}
	return nil
}

func JsonValueMapUnmarshal(data []byte, v *map[string]*VMValue) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	err := d.Decode(v)
	if err == nil {
		for key, val := range *v {
			VMValueConvert(val, v, key)
		}
	}
	return err
}

func GetRandomFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func SetCardType(mctx *MsgContext, curType string) {
	VarSetValueStr(mctx, "$cardType", curType)
}

func ReadCardType(mctx *MsgContext, curType string) string {
	var extra string
	var cardType string
	vCardType, exists := VarGetValue(mctx, "$cardType")
	if exists {
		cardType = vCardType.ToString()
	} else {
		cardType = "coc7"
	}
	if cardType != curType {
		extra = fmt.Sprintf("\n这似乎是一张 %s 人物卡，请确认当前游戏类型", cardType)
	}
	return extra
}

// GetCtxProxyFirst 等着后续再加 @名字 版本，以及 @team 版本 // pos int
func GetCtxProxyFirst(ctx *MsgContext, cmdArgs *CmdArgs, setTempVar bool) (*MsgContext, bool) {
	for _, i := range cmdArgs.At {
		if i.UserId == ctx.EndPoint.UserId {
			continue
		}

		if ctx.Group != nil {
			isBot := false
			for botUid, _ := range ctx.Group.BotList {
				if i.UserId == botUid {
					isBot = true
					break
				}
			}
			if isBot {
				continue
			}
		}

		//theChoosen := i.UID
		mctx, exists := i.CopyCtx(ctx)
		if exists {
			if mctx.Player != ctx.Player {
				mctx.LoadPlayerGroupVars(mctx.Group, mctx.Player)
				if setTempVar {
					SetTempVars(mctx, "???")
				}
			}
		}
		return mctx, exists
	}
	return ctx, false
}

func UserIdExtract(uid string) string {
	index := strings.Index(uid, ":")
	if index != -1 {
		return uid[index+1:]
	}
	return uid
}

func LockFreeMapToMap(m lockfree.HashMap) map[string]interface{} {
	ret := map[string]interface{}{}
	_ = m.Iterate(func(_k interface{}, _v interface{}) error {
		k, ok := _k.(string)
		if ok {
			ret[k] = _v
		}
		return nil
	})

	return ret
}

var SVG_ICON = []byte(`<svg id="Layer_1" data-name="Layer 1" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512"><defs><style>.cls-1{fill:#bdccd4;}.cls-2{fill:#fff;}</style></defs><title>icon-s</title><rect class="cls-1" x="6.36" y="7.14" width="498.86" height="498.86" rx="12"/><polygon class="cls-2" points="130.65 58.2 31.9 269.01 183.55 462.82 365.95 427.73 480.24 237.52 336.41 52 130.65 58.2"/><path d="M488.44,230.71,346.28,44.41h0a10.57,10.57,0,0,0-8.87-4.18L133.26,48.62h0a10.61,10.61,0,0,0-9.15,6L22.06,263h0a10.57,10.57,0,0,0,1.14,11.14l150.59,194.6a10.58,10.58,0,0,0,10.53,3.95L373.17,436l1.35-.46a10.59,10.59,0,0,0,5.72-4.71L489.16,242.44a10.6,10.6,0,0,0-.72-11.73ZM186,449.75l-24.1-187.3L385.9,376ZM364.21,107.21,159.67,244,140.55,72.87ZM149.65,248.53l-102.77,12L131.65,87.9ZM392.46,367.38,165.87,252.6l207-138.45,1.2,1.54,18.83,250.94ZM358.79,95.63,178.51,67.86,333,61.44ZM47.71,271.08l103.1-12L173.2,433.22ZM364.32,416l-120,23.36,135.29-49.82Zm38.14-65.88-16.62-219L467.21,238Z"/><polygon class="cls-2" points="157.03 220.4 160.14 249.69 178.19 258.84 374.4 120.32 373.55 108.64 358.48 106.33 157.03 220.4"/><path d="M297.84,193.19h0c-11-3.95-22.25-3.44-29.35,1.3-7.73,3.69-13.91,13-16.18,24.53C249,235.69,255,250.45,266,252.61c9.44,1.87,19.48-6.76,24-20.27,8.76,1.95,17,1,22.68-2.23a15,15,0,0,0,7-7.93C323.42,211.65,313.84,198.92,297.84,193.19Z"/><path d="M221.27,164c-8.94-3.2-18.77-2.18-27.68,2.88l-.08,0a44.16,44.16,0,0,0-19.37,23.68c-7.61,21.25,1.15,43.9,19.53,50.47,8.94,3.2,18.77,2.18,27.68-2.88l.08,0A44.16,44.16,0,0,0,240.8,214.5C248.41,193.25,239.65,170.61,221.27,164Z"/><ellipse class="cls-2" cx="194.6" cy="193" rx="21.33" ry="16.31" transform="translate(-62.71 287.4) rotate(-64.91)"/><circle class="cls-2" cx="225.91" cy="185.74" r="9.96"/><path d="M310.56,113.25a44.14,44.14,0,0,0-30.26,4.47,32.67,32.67,0,0,0-16.76,22.33c-3.78,19.15,11.16,38.29,33.3,42.66a44.15,44.15,0,0,0,30.26-4.47l.08-.05c8.92-5.06,14.84-13,16.68-22.28C347.64,136.76,332.7,117.62,310.56,113.25Z"/><ellipse class="cls-2" cx="286.98" cy="140.6" rx="21.33" ry="16.31" transform="translate(37.95 340.88) rotate(-64.91)"/><circle class="cls-2" cx="320.22" cy="132.25" r="9.96"/><ellipse cx="226.67" cy="154.45" rx="6.5" ry="9.75" transform="translate(-7.12 297.91) rotate(-65.8)"/><ellipse cx="252.33" cy="140.56" rx="9.75" ry="6.5" transform="translate(83.38 374.84) rotate(-83.32)"/></svg>`)

var HelpMasterInfoDefault = "骰主很神秘，什么都没有说——"
var HelpMasterLicenseDefault = "请在遵守以下规则前提下使用:\n" +
	"1. 遵守国家法律法规\n" +
	"2. 在跑团相关群进行使用\n" +
	"3. 不要随意踢出、禁言、刷屏\n" +
	"4. 务必信任骰主，有事留言\n" +
	"如不同意使用.bot bye使其退群，谢谢。\n" +
	"祝玩得愉快。"

var SimpleCocSuccessRankToText = map[int]string{
	-2: "大失败",
	-1: "失败",
	1:  "成功",
	2:  "困难成功",
	3:  "极难成功",
	4:  "大成功",
}

var SetCocRulePrefixText = map[int]string{
	0:  `0`,
	1:  `1`,
	2:  `2`,
	3:  `3`,
	4:  `4`,
	5:  `5`,
	11: `DeltaGreen`,
}

var SetCocRuleText = map[int]string{
	0:  "出1大成功\n不满50出96-100大失败，满50出100大失败(COC7规则书)",
	1:  "不满50出1大成功，满50出1-5大成功\n不满50出96-100大失败，满50出100大失败",
	2:  "出1-5且判定成功为大成功\n出96-100且判定失败为大失败",
	3:  "出1-5大成功\n出96-100大失败(即大成功/大失败时无视判定结果)",
	4:  "出1-5且≤(成功率/10)为大成功\n不满50出>=96+(成功率/10)为大失败，满50出100大失败",
	5:  "出1-2且≤(成功率/5)为大成功\n不满50出96-100大失败，满50出99-100大失败",
	11: "出1或检定成功基础上个位十位相同为大成功\n出100或检定失败基础上个位十位相同为大失败\n此规则无困难成功或极难成功",
}
