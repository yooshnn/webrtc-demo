PROTO_DIR = proto
GO_OUT_DIR = proto
PYTHON_OUT_DIR = apps/ai
PROTO_FILE = $(PROTO_DIR)/media.proto

.PHONY: all proto clean

proto: proto-go proto-python
	@echo "Protobuf files generated."

proto-go:
	@echo "Generating Go protobuf files..."
	@protoc --proto_path=$(PROTO_DIR) \
		--go_out=$(GO_OUT_DIR) --go_opt=paths=source_relative \
		--go-grpc_out=$(GO_OUT_DIR) --go-grpc_opt=paths=source_relative \
		$(PROTO_FILE)

proto-python:
	@echo "Generating Python protobuf files..."
	@python3 -m grpc_tools.protoc -I$(PROTO_DIR) \
		--python_out=$(PYTHON_OUT_DIR) --grpc_python_out=$(PYTHON_OUT_DIR) \
		$(PROTO_FILE)

clean:
	rm -f $(GO_OUT_DIR)/*.pb.go
	rm -f $(PYTHON_OUT_DIR)/*_pb2.py
	rm -f $(PYTHON_OUT_DIR)/*_pb2_grpc.py
	@echo "Cleaned generated files."
