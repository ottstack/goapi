package goapi

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"reflect"
	"strings"

	"github.com/fasthttp/websocket"
	"github.com/go-errors/errors"
	"github.com/kelseyhightower/envconfig"
	"github.com/ottstack/goapi/pkg/ecode"
	"github.com/ottstack/goapi/pkg/middleware"
	"github.com/valyala/fasthttp"
	"go.uber.org/automaxprocs/maxprocs"
)

type Server struct {
	methods       map[string]methodFactory
	streamMethods map[string]bool
	api           *openapi
	middlewares   []middleware.Middleware
	ctx           context.Context
	cancelFunc    context.CancelFunc
	addr          string
	swaggerPath   string
	apiContent    []byte

	rawHandler map[string]func(*fasthttp.RequestCtx)

	crossDomain bool
}

type serveConfig struct {
	Addr        string
	HomePath    string
	CrossDomain bool
}

type methodFactory func() (middleware.MethodFunc, interface{}, interface{})
type methodInfo struct {
	methodValue reflect.Value
	methodType  reflect.Type
	methodName  string

	tags    []string
	summary string

	factory     methodFactory
	reqType     reflect.Type
	rspType     reflect.Type
	path        string
	isWebsocket bool
}

func NewServer() *Server {
	cfg := &serveConfig{
		Addr:        "127.0.0.1:8081",
		HomePath:    "/api/",
		CrossDomain: false,
	}
	err := envconfig.Process("SERVE", cfg)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	sv := &Server{
		swaggerPath:   cfg.HomePath,
		addr:          cfg.Addr,
		ctx:           ctx,
		crossDomain:   cfg.CrossDomain,
		cancelFunc:    cancelFunc,
		methods:       make(map[string]methodFactory),
		streamMethods: make(map[string]bool),
		rawHandler:    make(map[string]func(*fasthttp.RequestCtx)),
	}
	sv.api = newOpenapi(cfg.HomePath)
	sv.api.parseType("", reflect.TypeOf(&ecode.APIError{}))
	return sv
}

func (s *Server) HandleRaw(path string, function func(*fasthttp.RequestCtx)) error {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if _, ok := s.methods[path]; ok {
		return errors.Errorf("function %s already registered", path)
	}
	if _, ok := s.rawHandler[path]; ok {
		return errors.Errorf("%s already registered for http handler", path)
	}
	s.rawHandler[path] = function
	return nil
}

func (s *Server) Use(m middleware.Middleware) *Server {
	s.middlewares = append(s.middlewares, m)
	return s
}

func (s *Server) Serve(services []interface{}) error {
	if err := s.parse(services); err != nil {
		return err
	}
	defer s.cancelFunc()
	// maxprocs
	maxprocs.Set(maxprocs.Logger(func(s string, args ...interface{}) {
		log.Printf(s, args...)
	}))

	showAddr := s.addr
	addrInfo := strings.SplitN(s.addr, ":", 2)
	if addrInfo[0] == "" || addrInfo[0] == "0" || addrInfo[0] == "0.0.0.0" {
		showAddr = "localhost:" + addrInfo[1]
	}
	log.Println("Serving API on http://" + showAddr + s.swaggerPath)
	s.apiContent = s.api.getOpenAPIV3()
	return fasthttp.ListenAndServe(s.addr, s.serve)
}

func (s *Server) parse(services []interface{}) error {
	for _, sv := range services {
		svType := reflect.TypeOf(sv)
		svValue := reflect.ValueOf(sv)
		if svValue.Kind() != reflect.Ptr || svValue.Elem().Kind() != reflect.Struct {
			return errors.Errorf("service paramter %s should be pointer to struct", svType)
		}
		for i := 0; i < svType.NumMethod(); i++ {
			m := svType.Method(i)
			path := s.swaggerPath + m.Name

			info := &methodInfo{
				path:        path,
				tags:        []string{svType.Elem().Name()},
				methodType:  m.Type,
				methodValue: svValue.MethodByName(m.Name),
				methodName:  m.Name,
			}
			if err := parseMethods(info); err != nil {
				return err
			}

			if info.isWebsocket {
				s.streamMethods[path] = true
			}
			s.methods[path] = info.factory

			s.api.addMethod(info)
		}
	}
	return nil
}

// serve as http handler
func (s *Server) serve(fastReq *fasthttp.RequestCtx) {
	// serve openapi
	path := string(fastReq.Path())
	if path == s.swaggerPath+"api.json" {
		fastReq.Write(s.apiContent)
		return
	}
	if path == s.swaggerPath {
		fastReq.Response.Header.Set("Content-Type", "text/html; charset=utf-8")
		fastReq.Write(s.api.getSwaggerHTML())
		return
	}
	if path == s.swaggerPath+"doc" {
		fastReq.Response.Header.Set("Content-Type", "text/html; charset=utf-8")
		fastReq.Write(s.api.getDocHTML())
		return
	}

	method := strings.ToUpper(string(fastReq.Method()))
	if s.crossDomain {
		referer := string(fastReq.Referer())
		if u, _ := url.Parse(referer); u != nil {
			fastReq.Response.Header.Set("Access-Control-Allow-Origin", fmt.Sprintf("%s://%s", u.Scheme, u.Host))
		} else {
			fastReq.Response.Header.Set("Access-Control-Allow-Origin", "*")
		}
		fastReq.Response.Header.Set("Access-Control-Allow-Credentials", "true")
		fastReq.Response.Header.Set("Access-Control-Allow-Headers", "authorization, origin, content-type, accept")
		fastReq.Response.Header.Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS,DELETE,PUT")
		if method == "OPTIONS" {
			return
		}
	}

	hd, ok := s.rawHandler[path]
	if ok {
		hd(fastReq)
		return
	}

	// path to func
	factory, ok := s.methods[path]
	if !ok {
		writeErrResponse(fastReq, &ecode.APIError{Code: 404, Message: fmt.Sprintf("Request %s %s not found", method, path)})
		return
	}
	realMethod, req, rsp := factory()

	var reqBody []byte
	decoder := jsonDecoder
	isWebsocket := s.streamMethods[path]
	var stream *streamImp

	doCallFunc := func() {
		if len(reqBody) > 0 {
			if err := decoder(reqBody, req); err != nil {
				writeErrResponse(fastReq, &ecode.APIError{Code: 400, Message: "Decode request body failed: " + err.Error()})
				return
			}
		}

		ctx := context.Background()

		// Middleware
		for i := range s.middlewares {
			mware := s.middlewares[len(s.middlewares)-i-1]
			realMethod = func(mm middleware.MethodFunc) middleware.MethodFunc {
				return func(ctx context.Context, req, rsp interface{}) error {
					return mware(ctx, fastReq, mm, req, rsp)
				}
			}(realMethod)
		}
		err := realMethod(ctx, req, rsp)
		if isWebsocket {
			return
		}
		if err != nil {
			writeErrResponse(fastReq, err)
			return
		}

		fastReq.Response.Header.Set("Content-Type", "application/json")
		rspBody, err := encoder(rsp)
		if err != nil {
			writeErrResponse(fastReq, errors.Errorf("marshal rsp error: %v", err))
			return
		}
		fastReq.Write(rspBody)
	}

	if isWebsocket {
		err := upgrader.Upgrade(fastReq, func(conn *websocket.Conn) {
			stream = rsp.(*streamImp)
			stream.conn = conn
			defer stream.close()
			doCallFunc()
		})
		if err != nil {
			log.Println("Upgrade websocket error: ", err.Error())
		}
		return
	} else {
		reqBody = fastReq.PostBody()
	}
	doCallFunc()
}

func parseMethods(m *methodInfo) error {
	method := m.methodType
	if method.NumIn() != 4 {
		return errors.Errorf("the number of argment in %s should be 3 instand of %d", m.path, method.NumIn()-1)
	}
	if method.NumOut() != 1 {
		return errors.Errorf("the number of return value in %s should be 1 instand of %d", m.path, method.NumOut())
	}

	ctx := method.In(1)
	req := method.In(2)
	rsp := method.In(3)

	if ctx.PkgPath() != "context" || ctx.Name() != "Context" {
		return errors.Errorf("first argment in %s should be context.Context", m.path)
	}

	if strings.HasPrefix(m.methodName, "Stream") {
		if req.Kind() != reflect.Interface || req.Name() != "RecvStream" {
			return errors.Errorf("the type of third argment in %s should be websocket.RecvStream", m.path)
		}
		if rsp.Kind() != reflect.Interface || rsp.Name() != "SendStream" {
			return errors.Errorf("the type of third argment in %s should be websocket.SendStream", m.path)
		}
		m.isWebsocket = true
	} else {
		if req.Kind() != reflect.Ptr || req.Elem().Kind() != reflect.Struct {
			return errors.Errorf("the type of second argment in %s should be pointer to struct", m.path)
		}
		if rsp.Kind() != reflect.Ptr || rsp.Elem().Kind() != reflect.Struct {
			return errors.Errorf("the type of third argment in %s should be pointer to struct", m.path)
		}
	}

	ret := method.Out(0)
	if ret.PkgPath() != "" || ret.Name() != "error" {
		return errors.Errorf("return type in %s should be error", m.path)
	}

	callFunc := func(ctx context.Context, req, rsp interface{}) error {
		args := []reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(req), reflect.ValueOf(rsp)}
		retValues := m.methodValue.Call(args)
		ret := retValues[0].Interface()
		if ret != nil {
			// ingore close error message
			if _, ok := ret.(*websocket.CloseError); ok {
				return nil
			}
			return ret.(error)
		}
		return nil
	}

	m.factory = func() (middleware.MethodFunc, interface{}, interface{}) {
		var rspVal, reqVal interface{}
		if m.isWebsocket {
			reqVal = &streamImp{}
			rspVal = reqVal
		} else {
			reqVal = reflect.New(req.Elem()).Interface()
			rspVal = reflect.New(rsp.Elem()).Interface()
		}
		return callFunc, reqVal, rspVal
	}
	if m.isWebsocket {
		m.reqType = req
		m.rspType = rsp
	} else {
		m.reqType = req.Elem()
		m.rspType = rsp.Elem()
	}
	return nil
}
