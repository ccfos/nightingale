/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements. See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership. The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License. You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package thrift

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type THttpClient struct {
	response           *http.Response
	url                *url.URL
	requestBuffer      *bytes.Buffer
	header             http.Header
	nsecConnectTimeout int64
	nsecReadTimeout    int64
}

type THttpClientTransportFactory struct {
	url    string
	isPost bool
}

func (p *THttpClientTransportFactory) GetTransport(trans TTransport) TTransport {
	if trans != nil {
		t, ok := trans.(*THttpClient)
		if ok && t.url != nil {
			if t.requestBuffer != nil {
				t2, _ := NewTHttpPostClient(t.url.String())
				return t2
			}
			t2, _ := NewTHttpClient(t.url.String())
			return t2
		}
	}
	if p.isPost {
		s, _ := NewTHttpPostClient(p.url)
		return s
	}
	s, _ := NewTHttpClient(p.url)
	return s
}

func NewTHttpClientTransportFactory(url string) *THttpClientTransportFactory {
	return &THttpClientTransportFactory{url: url, isPost: false}
}

func NewTHttpPostClientTransportFactory(url string) *THttpClientTransportFactory {
	return &THttpClientTransportFactory{url: url, isPost: true}
}

func NewTHttpClient(urlstr string) (TTransport, error) {
	parsedURL, err := url.Parse(urlstr)
	if err != nil {
		return nil, err
	}
	response, err := http.Get(urlstr)
	if err != nil {
		return nil, err
	}
	return &THttpClient{response: response, url: parsedURL}, nil
}

func NewTHttpPostClient(urlstr string) (TTransport, error) {
	parsedURL, err := url.Parse(urlstr)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 0, 1024)
	return &THttpClient{url: parsedURL, requestBuffer: bytes.NewBuffer(buf), header: http.Header{}}, nil
}

// Set the HTTP Header for this specific Thrift Transport
// It is important that you first assert the TTransport as a THttpClient type
// like so:
//
// httpTrans := trans.(THttpClient)
// httpTrans.SetHeader("User-Agent","Thrift Client 1.0")
func (p *THttpClient) SetHeader(key string, value string) {
	p.header.Add(key, value)
}

// Get the HTTP Header represented by the supplied Header Key for this specific Thrift Transport
// It is important that you first assert the TTransport as a THttpClient type
// like so:
//
// httpTrans := trans.(THttpClient)
// hdrValue := httpTrans.GetHeader("User-Agent")
func (p *THttpClient) GetHeader(key string) string {
	return p.header.Get(key)
}

// Deletes the HTTP Header given a Header Key for this specific Thrift Transport
// It is important that you first assert the TTransport as a THttpClient type
// like so:
//
// httpTrans := trans.(THttpClient)
// httpTrans.DelHeader("User-Agent")
func (p *THttpClient) DelHeader(key string) {
	p.header.Del(key)
}

func (p *THttpClient) Open() error {
	// do nothing
	return nil
}

func (p *THttpClient) IsOpen() bool {
	return p.response != nil || p.requestBuffer != nil
}

func (p *THttpClient) closeResponse() error {
	var err error
	if p.response != nil && p.response.Body != nil {
		err = p.response.Body.Close()
	}

	p.response = nil
	return err
}

func (p *THttpClient) Close() error {
	if p.requestBuffer != nil {
		p.requestBuffer.Reset()
		p.requestBuffer = nil
	}
	return p.closeResponse()
}

func (p *THttpClient) Read(buf []byte) (int, error) {
	if p.response == nil {
		return 0, NewTTransportException(NOT_OPEN, "Response buffer is empty, no request.")
	}
	n, err := p.response.Body.Read(buf)
	if n > 0 && (err == nil || err == io.EOF) {
		return n, nil
	}
	return n, NewTTransportExceptionFromError(err)
}

func (p *THttpClient) ReadByte() (c byte, err error) {
	return readByte(p.response.Body)
}

func (p *THttpClient) Write(buf []byte) (int, error) {
	n, err := p.requestBuffer.Write(buf)
	return n, err
}

func (p *THttpClient) WriteByte(c byte) error {
	return p.requestBuffer.WriteByte(c)
}

func (p *THttpClient) WriteString(s string) (n int, err error) {
	return p.requestBuffer.WriteString(s)
}

func (p *THttpClient) Flush() error {
	// Close any previous response body to avoid leaking connections.
	p.closeResponse()

	client := &http.Client{}
	req, err := http.NewRequest("POST", p.url.String(), p.requestBuffer)
	if err != nil {
		return NewTTransportExceptionFromError(err)
	}
	p.header.Add("Content-Type", "application/x-thrift")
	req.Header = p.header
	response, err := client.Do(req)
	if err != nil {
		return NewTTransportExceptionFromError(err)
	}
	if response.StatusCode != http.StatusOK {
		// Close the response to avoid leaking file descriptors.
		response.Body.Close()
		// TODO(pomack) log bad response
		return NewTTransportException(UNKNOWN_TRANSPORT_EXCEPTION, "HTTP Response code: "+strconv.Itoa(response.StatusCode))
	}
	p.response = response
	return nil
}

func (p *THttpClient) RemainingBytes() (num_bytes uint64) {
	len := p.response.ContentLength 
	if len >= 0 {
		return uint64(len)
	}
	
	const maxSize = ^uint64(0)
	return maxSize  // the thruth is, we just don't know unless framed is used
}

