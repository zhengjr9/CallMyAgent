.PHONY: all build build-server build-worker docker docker-server docker-worker deploy clean

REGISTRY ?= docker.io/library
VERSION  ?= latest

all: build docker

# ── Build ──────────────────────────────────────────
build: build-server build-worker

build-server:
	@echo "Building server binary..."
	cd backend && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
		go build -ldflags="-s -w" -o ../build/server .
	@echo "Server binary: build/server"

build-worker:
	@echo "Worker image uses container/ directly (no Go build needed)"

# ── Docker ─────────────────────────────────────────
docker: docker-server docker-worker

docker-server:
	@echo "Building server Docker image..."
	docker build -t $(REGISTRY)/callmyagent-server:$(VERSION) \
		-f docker/Dockerfile.server .
	@echo "Image: $(REGISTRY)/callmyagent-server:$(VERSION)"

docker-worker:
	@echo "Building worker Docker image..."
	docker build -t $(REGISTRY)/callmyagent-worker:$(VERSION) \
		-f container/Dockerfile container/
	@echo "Image: $(REGISTRY)/callmyagent-worker:$(VERSION)"

# ── Push ───────────────────────────────────────────
push: push-server push-worker

push-server:
	docker push $(REGISTRY)/callmyagent-server:$(VERSION)

push-worker:
	docker push $(REGISTRY)/callmyagent-worker:$(VERSION)

# ── Deploy ─────────────────────────────────────────
deploy:
	@echo "Deploying to Kubernetes..."
	kubectl apply -f k8s/rbac.yaml
	kubectl apply -f k8s/secret.yaml
	kubectl apply -f k8s/pvc.yaml
	kubectl apply -f k8s/deployment.yaml
	@echo "Done. Check: kubectl get pods -l app=callmyagent"

# ── Run locally ────────────────────────────────────
run:
	cd backend && go run .

run-docker:
	docker run --rm -p 8080:8080 \
		-e ANTHROPIC_API_KEY=$${ANTHROPIC_API_KEY} \
		-e KUBECONFIG=/root/.kube/config \
		-v $$HOME/.kube:/root/.kube:ro \
		$(REGISTRY)/callmyagent-server:$(VERSION)

# ── Clean ──────────────────────────────────────────
clean:
	rm -rf build/
	docker rmi $(REGISTRY)/callmyagent-server:$(VERSION) 2>/dev/null || true
	docker rmi $(REGISTRY)/callmyagent-worker:$(VERSION) 2>/dev/null || true
