SHELL := /bin/bash
GREEN=$(shell echo -e "\033[0;32m")
RED=$(shell echo -e "\033[0;31m")
YELLOW=$(shell echo -e "\033[0;33m")
NOCOLOR=$(shell echo -e "\033[0m")
HEADER=$(GREEN)Recipe:$(NOCOLOR)

# Add local tooling to path
export PATH:=$(PWD)/bin:$(PATH)
export PATH:=$(shell go env GOPATH)/bin:$(PATH)

include .env
export

.PHONY: help
default: help

help: ## show this help
	@echo '$(HEADER) help'
	@echo 'usage: make [target] ...'
	@echo ''
	@echo 'targets:'
	@egrep '^(.+)\:\ .*##\ (.+)' ${MAKEFILE_LIST} | sed 's/:.*##/#/' | column -t -c 2 -s '#'

.PHONY: start
start: ## start all services
	@echo '$(HEADER) start'
	docker-compose -f docker-compose.cs.yml up

.PHONY: sl
sl: ## start start in local mode
	@echo '$(HEADER) start'
	docker-compose -f docker-compose.local.yml up


.PHONY: startd
startd: ## start all services as a daemon
	@echo '$(HEADER) startd'
	docker-compose up -d

.PHONY: logs
logs: ## tail all logs
	@echo '$(HEADER) logs'
	docker-compose logs -f

.PHONY: stop
stop: ## stop all services
	@echo '$(HEADER) stop'
	docker-compose down

.PHONY: build
build: ## build
	@echo '$(HEADER) build'
	@cd backend && docker build . -t talk2robots_backend:latest

.PHONY: test
test: ## test
	@echo '$(HEADER) test'
	pushd backend && go test -v ./... && popd

.PHONY: cleanup
cleanup: ## cleanup
	@echo '$(HEADER) cleanup'
	docker system prune -a

## Go Development Targets
.PHONY: tools
tools: ## Install development tools
	@echo '$(HEADER) tools'
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

.PHONY: fmt
fmt: ## Format Go code
	@echo '$(HEADER) fmt'
	@cd backend && gofmt -s -l -w .
	@cd backend && goimports -w . || true
	@cd backend && go fmt ./...

.PHONY: lint
lint: ## Run linters
	@echo '$(HEADER) lint'
	@cd backend && golangci-lint run || true

.PHONY: vet
vet: ## Run go vet
	@echo '$(HEADER) vet'
	@cd backend && go vet ./...

.PHONY: ensure-goimports
ensure-goimports:
	@which goimports > /dev/null || go install golang.org/x/tools/cmd/goimports@latest

.PHONY: tidy
tidy: ensure-goimports ## Clean up Go modules and format code
	@echo '$(HEADER) tidy'
	@cd backend && goimports -w .
	@cd backend && go fmt ./...
	@cd backend && go mod tidy

.PHONY: deps
deps: tidy ## Download and update Go dependencies
	@echo '$(HEADER) deps'
	@cd backend && go get -u ./...

.PHONY: clean
clean: ## Clean up build artifacts
	@echo '$(HEADER) clean'
	@cd backend && rm -rf bin/ tmp/
	@cd backend && go clean -cache -testcache -modcache

## Kubernetes
.PHONY: dologin
dologin: ## login to digital ocean
	@echo '$(HEADER) dologin'
	doctl auth init -t $(DO_PAT)
	doctl kubernetes cluster list
	doctl kubernetes cluster kubeconfig save k8s-do-sfo3-talk2robots-prod

.PHONY: klogin
klogin: ## login to k8s
	@echo '$(HEADER) klogin'
	doctl kubernetes cluster kubeconfig save k8s-do-sfo3-talk2robots-prod

.PHONY: klogs
klogs: ## tail app k8s logs
	@echo '$(HEADER) klogs'
	kubectl logs -f -n talk2robots -l app=backend --tail=100 --all-containers=true --ignore-errors=true --timestamps=true

.PHONY: klogsredis
klogsredis: ## tail redis k8s logs
	@echo '$(HEADER) klogsredis'
	kubectl logs -f -n stateful -l app=redis --tail=100 --all-containers=true --ignore-errors=true --timestamps=true

.PHONY: configure-helm
configure-helm: ## configure helm
	@echo '$(HEADER) configure-helm'
	helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
	helm repo update ingress-nginx
	helm search repo ingress-nginx
	helm repo add datadog https://helm.datadoghq.com
	helm repo update datadog
	helm search repo datadog

.PHONY: apply-ingress-resources
apply-ingress-resources: ## apply ingress resources
	@echo '$(HEADER) apply-ingress-resources'
	cat ./infra/ingress/resources.yaml | kubectl apply -f -

.PHONY: update-ingress
update-ingress: ## upgrade ingress controller
	helm upgrade ingress-nginx ingress-nginx/ingress-nginx --version 4.5.2 \
  	-n ingress-nginx \
		--values ./infra/ingress/helm-values.yaml

.PHONY: tail-ingress-logs
tail-ingress-logs: ## tail ingress logs
	kubectl logs -f -n ingress-nginx -l app.kubernetes.io/name=ingress-nginx --tail=100

.PHONY: grafana
grafana: ## open grafana
	@echo '$(HEADER) grafana'
	kubectl --namespace monitoring port-forward svc/kube-prom-stack-grafana 3000:80

.PHONY: redis
redis: ## open redis
	@echo '$(HEADER) redis'
	echo "redis-cli -P 6380"
	kubectl --namespace stateful port-forward svc/redis 6380:6379

.PHONY: update-datadog
update-datadog: ## upgrade datadog
	helm upgrade datadog-agent datadog/datadog \
  	-n default \
		--set MONGO_HOST=$(MONGO_HOST),MONGO_PASSWORD=$(DATADOG_MONGO_PASS) \
		--values ./infra/observability/datadog.yaml


.PHONY: mongo
mongo: ## remind to curl ifconfig.me and open mongo
	@echo '$(HEADER) mongo'
	echo $$(curl ifconfig.me)
	@echo 'remember to add the ip to the mongo whitelist at https://cloud.digitalocean.com/databases/'
	connection_string=$$(kubectl get secret --namespace default admin -o jsonpath="{.data.mongo}" | base64 --decode) && \
	mongo $${connection_string}