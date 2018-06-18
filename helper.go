package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	log "github.com/liudanking/goutil/logutil"
)

func repeatReadBody(resp *http.Response) ([]byte, error) {
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Warning("read http rsp body failed:%v", err)
		return nil, err
	}
	resp.Body = ioutil.NopCloser(bytes.NewReader(data))
	return data, nil
}

func jsonMarshalIndentToFile(fn string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(fn, data, 0666)
}
