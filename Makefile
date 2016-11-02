#all: push
all: controller

# 0.0 shouldn't clobber any release builds
TAG = 0.0
PREFIX = gcr.io/google_containers/netscaler-ingress

controller_linux: controller.go netscaler.go
	CGO_ENABLED=0 GOOS=linux godep go build -a -installsuffix cgo -ldflags '-w' -o controller 

controller: controller.go netscaler.go
	godep go build  -o controller 

#container: controller
#	docker build -t $(PREFIX):$(TAG) .

#push: container
#	gcloud docker push $(PREFIX):$(TAG)

clean:
	rm -f controller 
