package mian

import "net/http"

func sayhelloName(writer http.ResponseWriter, request *http.Request) {
	request.ParseForm() //解析参数
}
