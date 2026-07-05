package storage

import (
	"context"
)

func WriteCurrentState(ctx context.Context, path string, value any) error {
	data, err := sanitizedJSONBytes(value, true)
	if err != nil {
		return wrapError("current state encode", ErrorClassWrite, err)
	}
	data = append(data, '\n')
	if err := atomicWriteFile(ctx, path, data); err != nil {
		return wrapError("current state write", ErrorClassWrite, err)
	}
	return nil
}
