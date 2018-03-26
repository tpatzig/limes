/******************************************************************************
*
*  Copyright 2018 Stefan Majewsky <majewsky@gmx.net>
*
*  Licensed under the Apache License, Version 2.0 (the "License");
*  you may not use this file except in compliance with the License.
*  You may obtain a copy of the License at
*
*      http://www.apache.org/licenses/LICENSE-2.0
*
*  Unless required by applicable law or agreed to in writing, software
*  distributed under the License is distributed on an "AS IS" BASIS,
*  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
*  See the License for the specific language governing permissions and
*  limitations under the License.
*
******************************************************************************/

package schwift

import (
	"net/http"
	"net/textproto"
)

//Headers represents a set of request headers or response headers.
//
//Users will typically use one of the subtypes (AccountHeaders,
//ContainerHeaders, ObjectHeaders) instead, which provide type-safe access to
//well-known headers. The http.Header-like interface on this type can be used
//read and write arbitary headers. For example, the following calls are
//equivalent:
//
//	h := make(AccountHeaders)
//	h.Headers.Set("X-Account-Meta-Quota-Bytes", "1048576")
//	h.BytesUsedQuota().Set(1048576)
//
type Headers map[string]string

//Clear sets the value for the specified header to the empty string. When the
//Headers instance is then sent to the server with Update(), the server will
//delete the value for that header; cf. Del().
func (h Headers) Clear(key string) {
	h[textproto.CanonicalMIMEHeaderKey(key)] = ""
}

//Del deletes a key from the Headers instance. When the Headers instance is
//then sent to the server with Update(), a key which has been deleted with
//Del() will remain unchanged on the server.
//
//For most writable attributes, a key which has been deleted with Del() will
//remain unchanged on the server. To remove the key on the server, use Clear()
//instead.
//
//For object metadata (but not other object attributes), deleting a key will
//cause that key to be deleted on the server. Del() is identical to Clear() in
//this case.
func (h Headers) Del(key string) {
	delete(h, textproto.CanonicalMIMEHeaderKey(key))
}

//Get returns the value for the specified header.
func (h Headers) Get(key string) string {
	return h[textproto.CanonicalMIMEHeaderKey(key)]
}

//Set sets a new value for the specified header. Any existing value will be
//overwritten.
func (h Headers) Set(key, value string) {
	h[textproto.CanonicalMIMEHeaderKey(key)] = value
}

//ToHTTP converts this Headers instance into the equivalent http.Header
//instance. The return value is guaranteed to be non-nil.
func (h Headers) ToHTTP() http.Header {
	dest := make(http.Header, len(h))
	for k, v := range h {
		dest.Set(k, v)
	}
	return dest
}

//ToOpts wraps this Headers instance into a RequestOpts instance, so that it
//can be passed to Schwift's various request methods.
//
//	hdr := NewObjectHeaders()
//	hdr.ContentType().Set("image/png")
//	hdr.Metadata().Set("color", "blue")
//	obj.Upload(content, hdr.ToOpts())
//
func (h Headers) ToOpts() *RequestOptions {
	return &RequestOptions{Headers: h}
}

func headersFromHTTP(src http.Header) Headers {
	h := make(Headers, len(src))
	for k, v := range src {
		if len(v) > 0 {
			h.Set(k, v[0])
		}
	}
	return h
}
