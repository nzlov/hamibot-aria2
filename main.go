package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"strings"

	"context"
	"time"

	"strconv"

	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/nzlov/gorm"
	argo "github.com/zyxar/argo/rpc"
)

const (
	help = `Aria2 命令：
【/bind】：绑定Aria2
【/unbind】：取消绑定
【/down】：下载连接
【/status】：查看状态`

	bind_h = `请在命令后输入Aria2 RPC地址和密码。例如：
/bind https://example.com/rpc
或者
/bind https://example.com/rpc xxxx
`
	down_h = `请在命令后输入下载地址和名字。例如：
/down https://example.com/a.mp4
或者
/down https://example.com/a.mp4 b.mp4
`
)

var db *gorm.DB

func main() {
	var err error
	db, err = gorm.Open("sqlite3", "./hamibotaria2.db")
	if err != nil {
		panic(err)
	}
	db.AutoMigrate(new(User))
	db.LogMode(true)

	http.HandleFunc("/", serve)
	fmt.Println("开始")
	http.ListenAndServe(":6666", nil)
}

type hamibot struct {
	OpenID   string `json:"openid"`
	ClientID string `json:"clientid"`
	ChatID   string `json:"chatid"`
	Command  string `json:"command"`
	Text     string `json:"text"`
}

type User struct {
	gorm.Model

	OpenID   string `json:"openid" gorm:"index"`
	ClientID string `json:"clientid" gorm:"index"`
	ChatID   string `json:"chatid" gorm:"index"`

	RPC   string `json:"rpc" `
	Token string `json:"token"`
}

func resp(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"success","data":"` + content + `"}`))
}

func serve(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		resp(w, "读取数据错误")
		fmt.Println(err.Error())
		return
	}

	fmt.Println("新的请求:", string(body))
	h := hamibot{}
	if err = json.Unmarshal(body, &h); err != nil {
		resp(w, "解析数据错误")
		fmt.Println(err.Error())
		return
	}

	t := strings.TrimSpace(h.Text)
	switch h.Command {
	case "/bind":
		ts := strings.Split(t, " ")
		rpchost := ""
		token := ""
		switch len(ts) {
		case 2:
			rpchost = ts[0]
			token = ts[1]
		case 1:
			rpchost = ts[0]
		default:
			resp(w, bind_h)
			return
		}
		if rpchost == "" {
			resp(w, bind_h)
			return
		}
		c, err := argo.New(context.Background(), rpchost, token, time.Second*5, nil)
		if err != nil {
			resp(w, "验证rpc服务失败:"+err.Error())
			fmt.Println(err)
			return
		}
		info, err := c.GetVersion()
		if err != nil {
			resp(w, "验证rpc服务失败:"+err.Error())
			fmt.Println(err)
			return
		}
		u := User{
			OpenID:   h.OpenID,
			ClientID: h.ClientID,
			ChatID:   h.ChatID,
		}
		if err := db.Where(&u).First(&u).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				resp(w, "内部错误")
				fmt.Println(err)
				return
			}
		}
		u.RPC = rpchost
		u.Token = token
		if u.ID > 0 {
			if err := db.Save(&u).Error; err != nil {
				resp(w, "内部错误")
				fmt.Println(err)
				return
			}
		} else {
			if err := db.Create(&u).Error; err != nil {
				resp(w, "内部错误")
				fmt.Println(err)
				return
			}
		}
		resp(w, "绑定成功。"+info.Version)
	case "/unbind":
		if err := db.Model(User{}).Where("open_id = ? and client_id = ? and chat_id = ?", h.OpenID, h.ClientID, h.ChatID).Updates(map[string]interface{}{
			"rpc":   "",
			"token": "",
		}).Error; err != nil {
			resp(w, "内部错误")
			fmt.Println(err)
			return
		}
		resp(w, "解绑成功")
	case "/down":
		ts := strings.Split(t, " ")
		url := ""
		name := ""
		switch len(ts) {
		case 2:
			url = ts[0]
			name = ts[1]
		case 1:
			url = ts[0]
		default:
			resp(w, down_h)
			return
		}
		if url == "" {
			resp(w, down_h)
			return
		}
		u := User{
			OpenID:   h.OpenID,
			ClientID: h.ClientID,
			ChatID:   h.ChatID,
		}
		if err := db.Where(&u).First(&u).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				resp(w, "内部错误")
				fmt.Println(err)
				return
			}
			resp(w, "请先绑定")
			fmt.Println(err)
			return
		}
		if u.RPC == "" {
			resp(w, "请先绑定")
			fmt.Println(err)
			return
		}
		c, err := argo.New(context.Background(), u.RPC, u.Token, time.Second*5, nil)
		if err != nil {
			resp(w, "验证rpc服务失败:"+err.Error())
			fmt.Println(err)
			return
		}
		if name == "" {
			gid, err := c.AddURI(url)
			if err != nil {
				resp(w, "验证rpc服务失败")
				fmt.Println(err)
				return
			}
			resp(w, "添加成功。"+gid)
			return
		} else {
			gid, err := c.AddURI(url, map[string]interface{}{
				"out": name,
			})
			if err != nil {
				resp(w, "添加服务错误:"+err.Error())
				fmt.Println(err)
				return
			}
			resp(w, "添加成功。"+gid)
			return
		}

	case "/status":
		u := User{
			OpenID:   h.OpenID,
			ClientID: h.ClientID,
			ChatID:   h.ChatID,
		}
		if err := db.Where(&u).First(&u).Error; err != nil {
			if err != gorm.ErrRecordNotFound {
				resp(w, "内部错误")
				fmt.Println(err)
				return
			}
			resp(w, "请先绑定")
			fmt.Println(err)
			return
		}
		if u.RPC == "" {
			resp(w, "请先绑定")
			fmt.Println(err)
			return
		}
		c, err := argo.New(context.Background(), u.RPC, u.Token, time.Second*5, nil)
		if err != nil {
			resp(w, "验证rpc服务失败:"+err.Error())
			fmt.Println(err)
			return
		}
		infos, err := c.TellActive("gid", "files", "status", "totalLength", "completedLength", "downloadSpeed", "bittorrent")
		//infos, err := c.TellActive()
		if err != nil {
			resp(w, "获取信息失败:"+err.Error())
			fmt.Println(err)
			return
		}
		fmt.Printf("%+v\n", infos)
		strs := []string{}

		for _, v := range infos {
			strs = append(strs, v.Gid+"["+v.Status+"]")
			if v.BitTorrent.Info.Name != "" {
				strs = append(strs, v.BitTorrent.Info.Name)
			} else if len(v.Files) > 0 {
				strs = append(strs, v.Files[0].Path)
			}
			strs = append(strs, fmt.Sprintf("速度:%v,比例：%v/%v", b(v.DownloadSpeed), b(v.CompletedLength), b(v.TotalLength)))
		}
		str := strings.Join(strs, "\n")
		if len(str) > 2000 {
			str = str[:2000]
		}
		resp(w, str)
	case "":
		resp(w, help)
	default:
		resp(w, "不支持此命令")
		return
	}
}

var units = []string{"B", "K", "M", "G", "T", "P"}

func b(v string) string {
	i, err := strconv.Atoi(v)
	if err != nil {
		return v
	}
	f := float64(i)
	u := 0

	for f/1024 > 1 {
		u++
		f /= 1024
		if u > len(units) {
			return "-"
		}
	}
	return fmt.Sprintf("%.2f%v", f, units[u])
}
