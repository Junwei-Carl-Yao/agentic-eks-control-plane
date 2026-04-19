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

TF              := terraform
TF_ENV          ?= dev
TF_VAR_FILE     := envs/$(TF_ENV)/terraform.tfvars

IMAGE_REGISTRY ?= ghcr.io/your-org
IMAGE_TAG      ?= dev

UV ?= uv

.PHONY: help \
        bootstrap sync \
        plan apply destroy \
        dev dev-backend dev-frontend \
        test test-backend test-frontend \
        lint lint-backend lint-frontend lint-terraform \
        format format-backend format-frontend format-terraform \
        backend frontend \
        deploy \
        drift teardown-verify \
        clean

help:
	@echo "Available targets:"
	@echo "  bootstrap          - provision Terraform remote state (S3 + DynamoDB)"
	@echo "  sync               - install backend deps via uv (creates .venv, writes uv.lock)"
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
	@echo "  teardown-verify    - destroy and verify no orphaned AWS resources"
	@echo ""
	@echo "Override the environment with TF_ENV=<name> (default: dev)."

ifeq ($(OS),Windows_NT)
bootstrap:
	@powershell -NoProfile -Command "$$p = '$(BOOTSTRAP)'; if (!(Test-Path $$p)) { Write-Host ('[placeholder] ' + $$p + ' is empty - implement remote-state bootstrap'); exit 0 }; $$lines = Get-Content $$p; $$meaningful = $$lines | Where-Object { $$_.Trim() -ne '' -and -not $$_.TrimStart().StartsWith('#') -and -not $$_.TrimStart().StartsWith('#!') }; if (@($$meaningful).Count -eq 0) { Write-Host ('[placeholder] ' + $$p + ' is empty - implement remote-state bootstrap'); exit 0 }; if (Get-Command bash -ErrorAction SilentlyContinue) { bash $$p } else { Write-Error \"bash is required to run scripts/bootstrap.sh\"; exit 1 }"
else
bootstrap:
	@if [ -s "$(BOOTSTRAP)" ] && grep -Eq '^[[:space:]]*[^#[:space:]]' "$(BOOTSTRAP)"; then \
	  bash "$(BOOTSTRAP)"; \
	else \
	  echo "[placeholder] $(BOOTSTRAP) is empty - implement remote-state bootstrap"; \
	fi
endif

sync:
	cd $(BACKEND_DIR) && $(UV) sync --extra dev

plan:
	cd $(INFRA_DIR) && $(TF) init -backend=true -input=false && $(TF) plan -var-file=$(TF_VAR_FILE) -out=tfplan

apply:
	cd $(INFRA_DIR) && $(TF) init -backend=true -input=false && $(TF) apply -var-file=$(TF_VAR_FILE) -auto-approve

destroy:
	cd $(INFRA_DIR) && $(TF) init -backend=true -input=false && $(TF) destroy -var-file=$(TF_VAR_FILE) -auto-approve

dev: dev-backend dev-frontend

dev-backend:
	cd $(BACKEND_DIR) && $(UV) run uvicorn app.main:app --reload

dev-frontend:
	@echo "[placeholder] frontend dev server (npm run dev)"

test: test-backend test-frontend

test-backend:
	cd $(BACKEND_DIR) && $(UV) run pytest

test-frontend:
	@echo "[placeholder] frontend tests (vitest)"

lint: lint-backend lint-frontend lint-terraform

lint-backend:
	cd $(BACKEND_DIR) && $(UV) run ruff check .
	cd $(BACKEND_DIR) && $(UV) run black --check .
	cd $(BACKEND_DIR) && $(UV) run mypy

lint-frontend:
	cd $(FRONTEND_DIR) && npm run lint
	cd $(FRONTEND_DIR) && npm run format:check
	cd $(FRONTEND_DIR) && npm run typecheck

lint-terraform:
	cd $(INFRA_DIR) && $(TF) fmt -check -recursive
	cd $(INFRA_DIR) && tflint --config="$(CURDIR)/.tflint.hcl"

format: format-backend format-frontend format-terraform

format-backend:
	cd $(BACKEND_DIR) && $(UV) run ruff check --fix .
	cd $(BACKEND_DIR) && $(UV) run black .

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

teardown-verify:
	@echo "[placeholder] teardown verification (scan for orphan ALBs/ENIs/EBS/IAM)"

ifeq ($(OS),Windows_NT)
clean:
	@if exist "$(subst /,\,$(INFRA_DIR)\tfplan)" del /Q "$(subst /,\,$(INFRA_DIR)\tfplan)"
else
clean:
	@rm -f "$(INFRA_DIR)/tfplan"
endif
