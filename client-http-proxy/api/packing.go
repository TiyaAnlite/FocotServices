package api

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"k8s.io/klog/v2"
)

// PrepareUnPackedResponse 返回包数据预处理
func PrepareUnPackedResponse(response *ProxyPackedResponse) (*ProxyResponse, error) {
	payload := response.Payload
	if response.Gzip {
		reader, err := gzip.NewReader(bytes.NewReader(response.Payload))
		defer reader.Close()
		if err != nil {
			klog.Errorf("Error at reading gzip data: %s", err.Error())
			return nil, err
		}
		var buffer bytes.Buffer
		_, err = io.Copy(&buffer, reader)
		if err != nil {
			klog.Errorf("Error at reading gzip data: %s", err.Error())
			return nil, err
		}
		payload = buffer.Bytes()
	}
	var resp ProxyResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		klog.Errorf("Error at unmarshal payload: %s", err.Error())
		return nil, err
	}

	return &resp, nil
}
