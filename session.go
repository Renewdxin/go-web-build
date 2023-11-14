package main

import (
	"encoding/base64"
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type Session interface {
	Set(key, value interface{}) error
	Get(value interface{}) interface{}
	Delete(key interface{}) error
	SessionID() string
}

type Provider interface {
	SessionInit(sid string) (Session, error)
	SessionRead(sid string) (Session, error)
	SessionDestory(sid string) error
	SessionGC(maxLifeTime int64)
}

type Manager struct {
	cookieName  string
	lock        sync.RWMutex
	provider    Provider
	maxLifeTime int64
}

var globalSessions *Manager
var provides = make(map[string]Provider)

func NewManager(provideName, cookieName string, maxLifeTime int64) (*Manager, error) {
	provider, ok := provides[provideName]
	if !ok {
		return nil, fmt.Errorf("session:unknown provide %q (forgotten imports?)", provideName)
	}
	return &Manager{provider: provider, cookieName: cookieName, maxLifeTime: maxLifeTime}, nil
}

func init() {
	globalSessions, _ = NewManager("memory", "gosessionid", 3600)
	go globalSessions.GC()
}

func Register(name string, provider Provider) {
	if provider == nil {
		panic("session: Register provider is nil")
	}
	if _, dup := provides[name]; dup {
		panic("session: Register called twice for provider" + name)
	}
	provides[name] = provider
}

func (manager *Manager) sessionId() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(b)
}

func (manager *Manager) SessionStart(w http.ResponseWriter, r *http.Request) (session Session) {
	// 获取互斥锁
	manager.lock.Lock()
	defer manager.lock.Unlock()

	// 获取cookie
	cookie, err := r.Cookie(manager.cookieName)
	if err != nil || cookie.Value == "" {
		// 生成新的session ID
		sid := manager.sessionId()
		// 初始化新的session
		session, _ = manager.provider.SessionInit(sid)
		// 创建cookie
		cookie := http.Cookie{Name: manager.cookieName, Value: url.QueryEscape(sid), Path: "/", HttpOnly: true, MaxAge: int(manager.maxLifeTime)}
		// 设置cookie
		http.SetCookie(w, &cookie)
	} else {
		// 解码已存在的session ID
		sid, _ := url.QueryUnescape(cookie.Value)
		// 读取session
		session, _ = manager.provider.SessionRead(sid)
	}
	return
}

func (manager *Manager) SessionDestroy(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(manager.cookieName)
	if err != nil || cookie.Value == "" {
		return
	} else {
		manager.lock.Lock()
		defer manager.lock.Lock()
		manager.provider.SessionDestory(cookie.Value)
		expiration := time.Now()
		cookie := http.Cookie{Name: manager.cookieName, Path: "/", HttpOnly: true, Expires: expiration, MaxAge: -1}
		http.SetCookie(w, &cookie)
	}
}

func count(w http.ResponseWriter, r *http.Request) {
	sess := globalSessions.SessionStart(w, r)
	createtime := sess.Get("createtime")

	if createtime == nil {
		sess.Set("createtime", time.Now().Unix())
	} else if (createtime.(int64) + 360) < (time.Now().Unix()) {
		globalSessions.SessionDestroy(w, r)
		sess = globalSessions.SessionStart(w, r)
	}

	ct := sess.Get("countnum")
	if ct == nil {
		sess.Set("countnum", 1)
	} else {
		sess.Set("countnum", (ct.(int) + 1))
	}
	t, _ := template.ParseFiles("count.gtpl")
	w.Header().Set("Content-Type", "text/html")
	t.Execute(w, sess.Get("countnum"))
}

func (manager *Manager) GC() {
	manager.lock.Lock()
	defer manager.lock.Unlock()
	time.AfterFunc(time.Duration(manager.maxLifeTime), func() {
		manager.provider.SessionGC(manager.maxLifeTime)
	})
}
