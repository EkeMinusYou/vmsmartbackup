IMAGE_NAME ?= vmsmartbackup
IMAGE_TAG ?= latest
REGISTRY ?=

.PHONY: all build run test clean image push

all: build

build:
	CGO_ENABLED=0 go build -o build/app .

run:
	go run .

test:
	go test -v -race ./...

clean:
	rm -rf build

image:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

push: image
	@if [ -z "$(REGISTRY)" ]; then echo "REGISTRY is not set"; exit 1; fi
	docker tag $(IMAGE_NAME):$(IMAGE_TAG) $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
	docker push $(REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)
