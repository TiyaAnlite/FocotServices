package api

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"k8s.io/klog/v2"
)

// PrepareUnPackedResponse 返回包数据预处理
func PrepareUnPackedResponse(response *ProxyPackedResponse) (*ProxyResponse, error) {
	payload := response.Payload
	if response.Gzip {
		buf := bytes.NewReader(response.Payload)
		reader, err := gzip.NewReader(buf)
		if err != nil {
			klog.Errorf("Error at reading gzip data: %s", err.Error())
			return nil, err
		}
		var b []byte
		_, err = reader.Read(b)
		if err != nil {
			klog.Errorf("Error at reading gzip data: %s", err.Error())
			return nil, err
		}
		payload = b
	}
	var resp ProxyResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		klog.Errorf("Error at unmarshal payload: %s", err.Error())
		return nil, err
	}

	return &resp, nil
}
