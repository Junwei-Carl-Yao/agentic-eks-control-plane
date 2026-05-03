# Top-level Makefile for the Agentic EKS Control Plane.

ifeq ($(OS),Windows_NT)
SHELL := cmd.exe
.SHELLFLAGS := /C
else
SHELL := /bin/bash
.SHELLFLAGS := -ec
endif

INFRA_DIR       := infrastructure
BACKEND_DIR     := backend
FRONTEND_DIR    := frontend
HELM_DIR        := deploy/helm
BOOTSTRAP       := scripts/bootstrap.sh
ASSERT_SCRIPT   := scripts/infra_assertions.sh
TEARDOWN_SCRIPT := scripts/teardown_verify.sh

TF              := terraform
TF_ENV          ?= dev
TF_VAR_FILE     := envs/$(TF_ENV)/terraform.tfvars
TF_BACKEND_FILE := envs/$(TF_ENV)/backend.hcl
TF_INIT         := $(TF) init -backend=true -input=false -backend-config=$(TF_BACKEND_FILE)

IMAGE_REGISTRY ?= ghcr.io/your-org
IMAGE_TAG      ?= dev

GO ?= go
GOFMT ?= gofmt

# On Windows, C:\Windows\System32\bash.exe (WSL) usually shadows Git Bash
# on PATH and fails when no WSL distro is installed. Resolve Git Bash
# explicitly via git's exec path so the verify scripts run from any shell.
ifeq ($(OS),Windows_NT)
GIT_EXEC_PATH := $(shell git --exec-path 2>nul)
ifeq ($(GIT_EXEC_PATH),)
BASH := bash
else
BASH := "$(GIT_EXEC_PATH)/../../../bin/bash.exe"
endif
else
BASH := bash
endif

.PHONY: help \
        bootstrap sync \
        plan apply destroy \
        dev dev-backend dev-frontend \
        test test-backend test-frontend \
        lint lint-backend lint-frontend lint-terraform \
        format format-backend format-frontend format-terraform \
        backend frontend \
        deploy \
        drift apply-verify teardown-verify \
        clean

help:
	@echo "Available targets:"
	@echo "  bootstrap          - provision Terraform remote state (S3 bucket; locking via S3 use_lockfile)"
	@echo "  sync               - download backend Go module deps (writes go.sum)"
	@echo "  plan               - terraform plan against env=$(TF_ENV)"
	@echo "  apply              - terraform apply against env=$(TF_ENV)"
	@echo "  destroy            - terraform destroy against env=$(TF_ENV)"
	@echo "  dev                - run backend and frontend dev servers"
	@echo "  test               - run backend + frontend test suites"
	@echo "  lint               - run all linters (backend + frontend + terraform)"
	@echo "  format             - auto-fix formatting across backend + frontend + terraform"
	@echo "  backend            - build and push the backend container image"
	@echo "  frontend           - build and push the frontend container image"
	@echo "  deploy             - helm upgrade --install backend + frontend charts"
	@echo "  drift              - run terraform plan -detailed-exitcode for drift"
	@echo "  apply-verify       - assert deployed infra matches spec (run after apply)"
	@echo "  teardown-verify    - scan AWS for orphans after destroy"
	@echo ""
	@echo "Override the environment with TF_ENV=<name> (default: dev)."

bootstrap:
	@$(BASH) "$(BOOTSTRAP)"

sync:
	cd $(BACKEND_DIR) && $(GO) mod download

plan:
	cd $(INFRA_DIR) && $(TF_INIT) && $(TF) plan -var-file=$(TF_VAR_FILE) -out=tfplan

apply:
	cd $(INFRA_DIR) && $(TF_INIT) && $(TF) apply -var-file=$(TF_VAR_FILE) -auto-approve

destroy:
	cd $(INFRA_DIR) && $(TF_INIT) && $(TF) destroy -var-file=$(TF_VAR_FILE) -auto-approve

dev: dev-backend dev-frontend

dev-backend:
	cd $(BACKEND_DIR) && $(GO) run ./cmd/server

dev-frontend:
	@echo "[placeholder] frontend dev server (npm run dev)"

test: test-backend test-frontend

test-backend:
	cd $(BACKEND_DIR) && $(GO) test ./...

test-frontend:
	@echo "[placeholder] frontend tests (vitest)"

lint: lint-backend lint-frontend lint-terraform

lint-backend:
	@$(BASH) "scripts/lint.sh"

lint-frontend:
	cd $(FRONTEND_DIR) && npm run lint
	cd $(FRONTEND_DIR) && npm run format:check
	cd $(FRONTEND_DIR) && npm run typecheck

lint-terraform:
	cd $(INFRA_DIR) && $(TF) fmt -check -recursive
	cd $(INFRA_DIR) && tflint --config="$(CURDIR)/.tflint.hcl"

format: format-backend format-frontend format-terraform

format-backend:
	cd $(BACKEND_DIR) && $(GOFMT) -w .

format-frontend:
	cd $(FRONTEND_DIR) && npm run format

format-terraform:
	cd $(INFRA_DIR) && $(TF) fmt -recursive

backend:
	@echo "[placeholder] backend image build + push ($(IMAGE_REGISTRY)/eks-control-plane-backend:$(IMAGE_TAG))"

frontend:
	@echo "[placeholder] frontend image build + push ($(IMAGE_REGISTRY)/eks-control-plane-frontend:$(IMAGE_TAG))"

deploy:
	@echo "[placeholder] helm upgrade --install backend + frontend charts"

drift:
	@echo "[placeholder] drift detection (terraform plan -detailed-exitcode -json)"

apply-verify:
	@$(BASH) "$(ASSERT_SCRIPT)"

teardown-verify:
	@$(BASH) "$(TEARDOWN_SCRIPT)"

ifeq ($(OS),Windows_NT)
clean:
	@if exist "$(subst /,\,$(INFRA_DIR)\tfplan)" del /Q "$(subst /,\,$(INFRA_DIR)\tfplan)"
else
clean:
	@rm -f "$(INFRA_DIR)/tfplan"
endif
