package main

/*
#include <stdint.h>
#include <stdlib.h>
typedef struct { void* ptr; size_t len; } cliproxy_buffer;
typedef int (*host_call_fn)(void*, const char*, const uint8_t*, size_t, cliproxy_buffer*);
typedef void (*host_free_fn)(void*, size_t);
typedef struct { uint32_t abi_version; void* host_ctx; host_call_fn call; host_free_fn free_buffer; } cliproxy_host_api;
typedef int (*plugin_call_fn)(char*, uint8_t*, size_t, cliproxy_buffer*);
typedef void (*plugin_free_fn)(void*, size_t);
typedef void (*plugin_shutdown_fn)(void);
typedef struct { uint32_t abi_version; plugin_call_fn call; plugin_free_fn free_buffer; plugin_shutdown_fn shutdown; } cliproxy_plugin_api;
extern int cliproxyPluginCall(char*, uint8_t*, size_t, cliproxy_buffer*);
extern void cliproxyPluginFree(void*, size_t);
extern void cliproxyPluginShutdown(void);
static cliproxy_host_api saved_host;
static void save_host(const cliproxy_host_api* host) { if(host) saved_host = *host; }
static int invoke_host(const char* m, const uint8_t* b, size_t n, cliproxy_buffer* out) {
  return saved_host.call ? saved_host.call(saved_host.host_ctx,m,b,n,out) : 1;
}
static void release_host(void* p,size_t n) { if(p && saved_host.free_buffer) saved_host.free_buffer(p,n); }
*/
import "C"

import (
	"encoding/json"
	"fmt"
	"unsafe"
)

func main() {}

type hostCaller interface{ Call(string, any, any) error }
type abiHost struct{}

func (abiHost) Call(method string, input, output any) error {
	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	m := C.CString(method)
	defer C.free(unsafe.Pointer(m))
	b := C.CBytes(payload)
	defer C.free(b)
	var out C.cliproxy_buffer
	status := C.invoke_host(m, (*C.uint8_t)(b), C.size_t(len(payload)), &out)
	if out.ptr != nil {
		defer C.release_host(out.ptr, out.len)
	}
	if out.ptr == nil || uint64(out.len) > 16<<20 {
		return fmt.Errorf("host callback unavailable: %s", method)
	}
	var response struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code       string `json:"code"`
			Message    string `json:"message"`
			HTTPStatus int    `json:"http_status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(C.GoBytes(out.ptr, C.int(out.len)), &response); err != nil {
		return fmt.Errorf("invalid host response: %s", method)
	}
	if status != 0 || !response.OK {
		if response.Error != nil {
			return fmt.Errorf("%s (HTTP %d): %s", response.Error.Code, response.Error.HTTPStatus, redact(response.Error.Message))
		}
		return fmt.Errorf("host callback failed: %s", method)
	}
	if output != nil {
		return json.Unmarshal(response.Result, output)
	}
	return nil
}

//export cliproxy_plugin_init
func cliproxy_plugin_init(host *C.cliproxy_host_api, plugin *C.cliproxy_plugin_api) C.int {
	if host == nil || plugin == nil || host.abi_version != 1 {
		return 1
	}
	C.save_host(host)
	plugin.abi_version = 1
	plugin.call = C.plugin_call_fn(C.cliproxyPluginCall)
	plugin.free_buffer = C.plugin_free_fn(C.cliproxyPluginFree)
	plugin.shutdown = C.plugin_shutdown_fn(C.cliproxyPluginShutdown)
	return 0
}

//export cliproxyPluginCall
func cliproxyPluginCall(method *C.char, request *C.uint8_t, n C.size_t, response *C.cliproxy_buffer) C.int {
	if response == nil {
		return 1
	}
	response.ptr = nil
	response.len = 0
	if method == nil || uint64(n) > 2<<20 {
		return 1
	}
	defer func() {
		if recover() != nil {
			writeABI(response, nil, fmt.Errorf("plugin internal error"))
		}
	}()
	var raw []byte
	if request != nil && n > 0 {
		raw = C.GoBytes(unsafe.Pointer(request), C.int(n))
	}
	result, err := dispatch(C.GoString(method), raw)
	writeABI(response, result, err)
	return 0
}

func writeABI(out *C.cliproxy_buffer, result any, err error) {
	v := map[string]any{"ok": err == nil, "result": result}
	if err != nil {
		v = map[string]any{"ok": false, "error": map[string]any{"code": "model_quality_error", "message": redact(err.Error())}}
	}
	b, e := json.Marshal(v)
	if e != nil {
		b = []byte(`{"ok":false,"error":{"code":"encode_error","message":"response encoding failed"}}`)
	}
	out.ptr = C.CBytes(b)
	out.len = C.size_t(len(b))
}

//export cliproxyPluginFree
func cliproxyPluginFree(ptr unsafe.Pointer, _ C.size_t) {
	if ptr != nil {
		C.free(ptr)
	}
}

//export cliproxyPluginShutdown
func cliproxyPluginShutdown() { shutdownEngine() }
