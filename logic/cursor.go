package logic

import (
	"encoding/base64"
	"encoding/json"
)

// 游标编解码辅助：opaque cursor = base64(json)，前端不解析、只透传

func base64Encode(raw []byte) string {
	return base64.RawURLEncoding.EncodeToString(raw)
}

func base64Decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

func marshalJSONMust(v interface{}) []byte {
	raw, err := json.Marshal(v)
	if err != nil {
		// 游标结构体不可序列化属于程序错误
		panic(err)
	}
	return raw
}

func unmarshalJSONMust(raw []byte, v interface{}) error {
	return json.Unmarshal(raw, v)
}
