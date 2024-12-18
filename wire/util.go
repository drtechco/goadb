package wire

import (
	"fmt"
	"io"
	"regexp"
	"sync"
	"encoding/binary"
	"github.com/zach-klippenstein/goadb/internal/errors"
)

// ErrorResponseDetails is an error message returned by the server for a particular request.
type ErrorResponseDetails struct {
	Request   string
	ServerMsg string
}

// deviceNotFoundMessagePattern matches all possible error messages returned by adb servers to
// report that a matching device was not found. Used to set the DeviceNotFound error code on
// error values.
//
// Old servers send "device not found", and newer ones "device 'serial' not found".
var deviceNotFoundMessagePattern = regexp.MustCompile(`device( '.*')? not found`)

func adbServerError(request string, serverMsg string) error {
	var msg string
	if request == "" {
		msg = fmt.Sprintf("server error: %s", serverMsg)
	} else {
		msg = fmt.Sprintf("server error for %s request: %s", request, serverMsg)
	}

	errCode := errors.AdbError
	if deviceNotFoundMessagePattern.MatchString(serverMsg) {
		errCode = errors.DeviceNotFound
	}

	return &errors.Err{
		Code:    errCode,
		Message: msg,
		Details: ErrorResponseDetails{
			Request:   request,
			ServerMsg: serverMsg,
		},
	}
}

// IsAdbServerErrorMatching returns true if err is an *Err with code AdbError and for which
// predicate returns true when passed Details.ServerMsg.
func IsAdbServerErrorMatching(err error, predicate func(string) bool) bool {
	if err, ok := err.(*errors.Err); ok && err.Code == errors.AdbError {
		return predicate(err.Details.(ErrorResponseDetails).ServerMsg)
	}
	return false
}

func errIncompleteMessage(description string, actual int, expected int) error {
	return &errors.Err{
		Code:    errors.ConnectionResetError,
		Message: fmt.Sprintf("incomplete %s: read %d bytes, expecting %d", description, actual, expected),
		Details: struct {
			ActualReadBytes int
			ExpectedBytes   int
		}{
			ActualReadBytes: actual,
			ExpectedBytes:   expected,
		},
	}
}

// writeFully writes all of data to w.
// Inverse of io.ReadFully().
func writeFully(w io.Writer, data []byte) error {
	offset := 0
	for offset < len(data) {
		n, err := w.Write(data[offset:])
		if err != nil {
			return errors.WrapErrorf(err, errors.NetworkError, "error writing %d bytes at offset %d", len(data), offset)
		}
		offset += n
	}
	return nil
}

// MultiCloseable wraps c in a ReadWriteCloser that can be safely closed multiple times.
func MultiCloseable(c io.ReadWriteCloser) io.ReadWriteCloser {
	return &multiCloseable{ReadWriteCloser: c}
}

type multiCloseable struct {
	io.ReadWriteCloser
	closeOnce sync.Once
	err       error
}

func (c *multiCloseable) Close() error {
	c.closeOnce.Do(func() {
		c.err = c.ReadWriteCloser.Close()
	})
	return c.err
}

func DecodeV2Data(data []byte, stdout, stderr io.Writer) (int, error) {
	var exitCode int
	offset := 0

	for offset < len(data) {
		// 每个数据包的最小长度是 5 字节：1 字节 packetId + 4 字节数据长度
		if offset+5 > len(data) {
			if stdout != nil {
				stdout.Write(data[offset:])
			}
			break
		}

		// 读取 packetId 和数据长度
		packetId := data[offset]
		dataLen := binary.LittleEndian.Uint32(data[offset+1 : offset+5])

		// 检查数据包是否完整
		if offset+5+int(dataLen) > len(data) {
			if stdout != nil {
				stdout.Write(data[offset:])
			}
			break
		}

		// 获取数据内容
		packetData := data[offset+5 : offset+5+int(dataLen)]
		offset += 5 + int(dataLen)

		// 根据 packetId 处理不同的内容
		switch packetId {
		case 1: // stdout
			if stdout != nil {
				if _, err := stdout.Write(packetData); err != nil {
					return exitCode, fmt.Errorf("failed to write to stdout: %v", err)
				}
			}
		case 2: // stderr
			if stderr != nil {
				if _, err := stderr.Write(packetData); err != nil {
					return exitCode, fmt.Errorf("failed to write to stderr: %v", err)
				}
			}
		case 3: // exit code
			if len(packetData) > 0 {
				exitCode = int(packetData[0])
			}
		default:
		 	return 0, errors.Errorf(errors.ParseError,"unknown packet ID: %d", packetId)
		}
	}

	return exitCode, nil
}

func DecodeDataFromReader(reader io.Reader, stdout, stderr io.Writer) (int, error) {
	var exitCode int
	offset := 0

	// 用于存储读取到的数据
	buf := make([]byte, 4096) // 可以根据需要调整缓冲区大小

	// 读取数据
	for {
		n, err := reader.Read(buf)
		if err != nil && err != io.EOF {
			return exitCode, fmt.Errorf("failed to read data: %v", err)
		}

		// 如果读取的数据为空并且已经是 EOF，退出
		if n == 0 && err == io.EOF {
			break
		}

		// 处理读取到的部分数据
		data := buf[:n]
		for offset < len(data) {
			// 每个数据包的最小长度是 5 字节：1 字节 packetId + 4 字节数据长度
			if offset+5 > len(data) {
				if stdout != nil {
					stdout.Write(data[offset:])
				}
				break
			}

			// 读取 packetId 和数据长度
			packetId := data[offset]
			dataLen := binary.LittleEndian.Uint32(data[offset+1 : offset+5])

			// 检查数据包是否完整
			if offset+5+int(dataLen) > len(data) {
				if stdout != nil {
					stdout.Write(data[offset:])
				}
				break
			}

			// 获取数据内容
			packetData := data[offset+5 : offset+5+int(dataLen)]
			offset += 5 + int(dataLen)

			// 根据 packetId 处理不同的内容
			switch packetId {
			case 1: // stdout
				if stdout != nil {
					if _, err := stdout.Write(packetData); err != nil {
						return exitCode, fmt.Errorf("failed to write to stdout: %v", err)
					}
				}
			case 2: // stderr
				if stderr != nil {
					if _, err := stderr.Write(packetData); err != nil {
						return exitCode, fmt.Errorf("failed to write to stderr: %v", err)
					}
				}
			case 3: // exit code
				if len(packetData) > 0 {
					exitCode = int(packetData[0])
				}
			default:
				return 0, errors.Errorf(errors.ParseError,"unknown packet ID: %d", packetId)
			}
		}

		// 如果已经到了 EOF，结束循环
		if err == io.EOF {
			break
		}
	}

	return exitCode, nil
}