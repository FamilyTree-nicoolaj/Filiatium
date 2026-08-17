BINAIRE     := filiatium
CORPUS      ?= $(HOME)/Documents/Généalogie
GED         ?= $(CORPUS)/family.ged
OUTILS_PY   ?= $(CORPUS)/outils
VERSION     := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.version=$(VERSION)
PLATEFORMES := darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64

.DEFAULT_GOAL := run
.PHONY: run help build install test couverture vet fmt verif parite roundtrip fusion distribution propre

run: build ## (défaut) Compile puis lance l'outil en mode guidé
	@./$(BINAIRE)

help: ## Affiche cette aide
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
	  | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-12s\033[0m %s\n",$$1,$$2}'

build: ## Compile le binaire local
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINAIRE) .

install: ## Installe dans $$GOBIN
	go install -trimpath -ldflags '$(LDFLAGS)' .

test: ## Lance tous les tests
	go test ./...

couverture: ## Rapport de couverture HTML
	go test -coverprofile=couverture.out ./... && go tool cover -html=couverture.out

vet: ## Analyse statique
	go vet ./...

fmt: ## Reformate le code
	gofmt -l -w .

verif: vet test ## Contrôle avant commit : format + vet + tests
	@test -z "$$(gofmt -l .)" || { echo 'à reformater :'; gofmt -l .; exit 1; }

parite: build ## RECETTE : compare les signalements Go et Python sur tout le corpus
	@scripts/parite.sh "$(OUTILS_PY)" "$(CORPUS)"

roundtrip: build ## Vérifie que charger+réécrire ne change pas un octet
	@scripts/roundtrip.sh "$(CORPUS)"

fusion: build ## Analyse la fusion de chaque arbre secondaire avec l'arbre principal
	@for a in "$(CORPUS)"/secondary_trees/*.ged; do \
	  echo "=== $$(basename $$a)"; ./$(BINAIRE) merge --analyse "$(GED)" "$$a" | tail -3; echo; \
	done

distribution: ## Compile pour macOS (+ binaire universel), Linux et Windows
	@mkdir -p dist
	@for p in $(PLATEFORMES); do \
	  os=$${p%/*}; arch=$${p#*/}; ext=''; \
	  [ "$$os" = windows ] && ext=.exe; \
	  echo "  → $$os/$$arch"; \
	  GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -trimpath \
	    -ldflags '$(LDFLAGS)' -o dist/$(BINAIRE)-$$os-$$arch$$ext . || exit 1; \
	done
	@if command -v lipo >/dev/null 2>&1; then \
	  echo "  → darwin/universal"; \
	  lipo -create -output dist/$(BINAIRE)-darwin-universal \
	    dist/$(BINAIRE)-darwin-arm64 dist/$(BINAIRE)-darwin-amd64; \
	else \
	  echo "  (lipo absent — binaire universel macOS non généré ; nécessite les outils en ligne de commande Xcode)"; \
	fi

propre: ## Supprime les artefacts
	rm -rf $(BINAIRE) dist couverture.out
