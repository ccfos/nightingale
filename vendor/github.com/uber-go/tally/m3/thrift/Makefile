thrift_version := v1.0.0

gen-thrift:
	@thrift --gen go:thrift_import="github.com/apache/thrift/lib/go/thrift" -out . $(thrift_version)/m3.thrift
	@rm -rf m3/m3-remote
	@mv m3/* .
	@rm -rf m3
	@echo Generated thrift go files in metrics/m3/thrift/

