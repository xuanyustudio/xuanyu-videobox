package linkdl

import (
	"fmt"
	"io"
)

func readBodyLimit(r io.Reader, limit int64) ([]byte, error) {
	lr := io.LimitReader(r, limit+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("响应体超过 %d 字节限制", limit)
	}
	return data, nil
}
