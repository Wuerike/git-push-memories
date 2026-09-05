# Spike: roots separados no agent-memory

## Decisao

**Resultado binario: passa. Disposicao revisada: candidato a componente.**

Na revisao fixa `51dac9bf6e72c67f6314ee3abb00515986a3fa4a`, um prototipo descartavel demonstrou que dois repositorios de codigo somente leitura podem consumir um repositorio de memoria compartilhado e gravavel. A evidencia foi preservada em `.artifacts/phase-1/20260905T170000Z-agent-memory-stage7-evidence`, conforme o [manifesto](../evidence-manifest.md).

O resultado remove a duvida sobre viabilidade de separar roots, mas nao promove o candidato a fork ou base. As falhas ja demonstradas em privacidade, persistencia de sessoes, staging mutavel, revalidacao no apply, drift e consumo do indice Git preexistente ainda exigem substituicao ou endurecimento.

## Pergunta e seam

A pergunta foi se `CODE_ROOT_A != MEMORY_ROOT` e `CODE_ROOT_B != MEMORY_ROOT` poderiam operar com Markdown, indice, staging, locks e mutacoes Git somente no repositorio de memoria, sem redesenhar o formato Markdown, o modelo de operacoes ou o schema SQLite.

O prototipo introduziu uma interface `Roots` na composicao do processo, resolvida por `AGENT_MEMORY_MEMORY_ROOT` e `AGENT_MEMORY_CONSUMER_ID`; a flag `--root` existente continuou representando o repositorio consumidor. O patch de arquivos rastreados esta em `prototype-final.patch`; os tres arquivos novos omitidos pelo `git diff` original foram preservados em `prototype-new-files/`. A revisao dessa diferenca de classificacao e completude esta em `review-notes.md`.

## Ambiente e reproducao

A origem `https://github.com/xChuCx/agent-memory.git` estava limpa em `main` e foi clonada localmente para uma copia destacada na revisao fixa. O run usou Linux x86_64, Git 2.54.0, Bubblewrap 0.6.1, strace 5.16 e Go 1.25.12. `HOME`, caches e configuracao Git eram descartaveis; os fluxos classificatorios executaram offline, com os dois code roots em `--ro-bind` e apenas prototipo, tool home, binario e memory root gravaveis. A unica preparacao com rede permitida foi `go mod download`; sua saida vazia indica que o cache local pode ter satisfeito a operacao.

```bash
env -i PATH="$PATH" HOME="$TOOL_HOME" GIT_CONFIG_GLOBAL=/dev/null \
  GIT_CONFIG_NOSYSTEM=1 git clone --local --no-hardlinks --no-checkout \
  /home/Wuerike.Cavalheiro/workspace/agent-memory "$RUN_ROOT/source-reference"
env -i PATH="$PATH" HOME="$TOOL_HOME" GIT_CONFIG_GLOBAL=/dev/null \
  GIT_CONFIG_NOSYSTEM=1 git -C "$RUN_ROOT/source-reference" checkout --detach \
  51dac9bf6e72c67f6314ee3abb00515986a3fa4a
# Mesmo wrapper de run_test, acrescentando --share-net somente nesta preparacao.
go mod download
```

O helper de testes montou a copia do prototipo como gravavel para o ciclo red-green; o helper comportamental montou os code roots somente leitura e a memoria como gravavel:

```bash
run_test() {
  bwrap --unshare-all --die-with-parent --new-session \
    --ro-bind /usr /usr --ro-bind /bin /bin --ro-bind /lib /lib \
    --ro-bind /lib64 /lib64 --proc /proc --dev /dev --tmpfs /tmp \
    --dir /opt --ro-bind "$GO_ROOT" /opt/go \
    --dir /home --bind "$RUN_ROOT/tool-home" /home/phase-1 \
    --dir /workspace --bind "$RUN_ROOT/prototype" /workspace/candidate \
    --bind "$RUN_ROOT/bin" /workspace/bin \
    --ro-bind "$RUN_ROOT/source-reference" /workspace/source-reference \
    --chdir /workspace/candidate --clearenv \
    --setenv HOME /home/phase-1 --setenv PATH /opt/go/bin:/usr/bin:/bin \
    --setenv TMPDIR /home/phase-1/tmp --setenv GOPATH /home/phase-1/go \
    --setenv GOMODCACHE /home/phase-1/go/pkg/mod \
    --setenv GOCACHE /home/phase-1/go-build --setenv GOENV off \
    --setenv GOTOOLCHAIN local --setenv GIT_CONFIG_GLOBAL /dev/null \
    --setenv GIT_CONFIG_NOSYSTEM 1 -- "$@"
}

run_behavior() {
  local code_root=$1 consumer_id=$2
  shift 2
  bwrap --unshare-all --die-with-parent --new-session \
    --ro-bind /usr /usr --ro-bind /bin /bin --ro-bind /lib /lib \
    --ro-bind /lib64 /lib64 --proc /proc --dev /dev --tmpfs /tmp \
    --dir /home --bind "$RUN_ROOT/tool-home" /home/phase-1 \
    --dir /workspace --ro-bind "$RUN_ROOT/bin" /workspace/bin \
    --ro-bind "$RUN_ROOT/code-a" /workspace/code-a \
    --ro-bind "$RUN_ROOT/code-b" /workspace/code-b \
    --bind "$RUN_ROOT/memory-repo" /workspace/memory \
    --chdir "/workspace/$code_root" --clearenv \
    --setenv HOME /home/phase-1 --setenv PATH /workspace/bin:/usr/bin:/bin \
    --setenv TMPDIR /home/phase-1/tmp --setenv LANG C.UTF-8 --setenv TZ UTC \
    --setenv GIT_CONFIG_GLOBAL /dev/null --setenv GIT_CONFIG_NOSYSTEM 1 \
    --setenv AGENT_MEMORY_MEMORY_ROOT /workspace/memory \
    --setenv AGENT_MEMORY_CONSUMER_ID "$consumer_id" -- "$@"
}
```

Os comandos centrais executados dentro deles foram:

```bash
go build -o /workspace/bin/agent-memory ./cmd/agent-memory
go test -tags=e2e ./internal/e2e -run '^TestStage7' -count=1
go test -tags=e2e ./internal/e2e -run '^TestStage7ConcurrentStagingIDsAreUnique$' -count=20
go test ./... -count=1
go test -race ./... -count=1
go test -tags=e2e ./internal/e2e/... -count=1

agent-memory init --name shared-checkout-memory
agent-memory fetch 'timeout checkout-api time responsavel' --json
agent-memory propose --intent refresh_module --op replace_section_content \
  --path modules/checkout-api.md --section-id checkout-api-operation \
  --source file:services/checkout-api/service.yaml --confidence confirmed \
  --rationale 'sync synthetic checkout evidence' --json
agent-memory review 20260905T202218-refresh-module-sync-synthetic-checkout-e --diff
agent-memory apply 20260905T202218-refresh-module-sync-synthetic-checkout-e --json
agent-memory rebuild-index --json
```

| Comando | Codigo | Duracao |
|---|---:|---:|
| build | 0 | 0,54 s |
| Stage 7 focado | 0 | 1,38 s |
| stress de staging, 20 repeticoes | 0 | 3,02 s |
| `go test ./...` | 0 | 30,53 s |
| `go test -race ./...` | 0 | 22,00 s |
| E2E oficial | 0 | 7,44 s |
| init | 0 | 0,13 s |
| fetch consumidor A/B | 0/0 | 0,04/0,05 s |
| propose | 0 | 0,03 s |
| review | 0 | 0,01 s |
| apply | 0 | 0,08 s |
| reaplicar staging consumido | 1 esperado | 0,01 s |
| fetch em processo novo | 0 | 0,04 s |
| rebuild | 0 | 0,03 s |
| fetch apos rebuild | 0 | 0,02 s |
| memory root inexistente | 1 esperado | 0,01 s |
| integridade SQLite | 0, `ok` | 0,06 s |

stdout, stderr e tempo de cada comando foram preservados separadamente. `pre-inventory.txt`, `post-inventory.txt`, `memory-final-*`, `code-*-final-*` e `source-reference-final-status.txt` registram os estados antes e depois; a origem e os code roots terminaram limpos.

## Resultados

| Verificacao | Resultado |
|---|---|
| CLI e MCP com roots separados | passa |
| dois consumidores sem cross-talk | passa |
| code roots somente leitura e sem mutacao Git | passa |
| memory root invalido sem fallback | passa |
| cold start concorrente, 20 repeticoes | passa |
| IDs de staging concorrentes | passa apos reserva atomica |
| applies concorrentes em arquivos distintos | passa |
| snapshots concorrentes do mesmo arquivo | segundo apply rejeitado por drift, sem perda |
| rebuild completo e recuperacao em processo novo | passa no prototipo |
| `PRAGMA integrity_check` | `ok` |
| `go test ./... -count=1` | passa em 30,53 s |
| `go test -race ./... -count=1` | passa em 22,00 s |
| E2E oficial com tag | passa em 7,44 s |

O fluxo classificado executou offline em Bubblewrap. O trace representativo registrou Git apenas como `git -C /workspace/memory`; commits, objetos e status dos dois repositorios consumidores permaneceram iguais ao baseline (`trace-git-execs.txt`; `code-*-confinement.sha256`). O estado final autoritativo continha tres segundos e `team-b`, sobreviveu a processo novo e a remocao/reconstrucao do indice (`behavior-fresh-fetch.stdout`; `behavior-rebuild.stdout`; `behavior-fetch-rebuilt.stdout`).

## Defeitos revelados

O ciclo red-green encontrou tres defeitos que precisaram de correcoes delimitadas no prototipo:

- inicializacao concorrente do SQLite produzia `SQLITE_BUSY` antes de usar o lock comum;
- IDs baseados em segundos colidiam para propostas simultaneas;
- dois snapshots do mesmo arquivo podiam aplicar com sucesso e perder silenciosamente a primeira mudanca.

O prototipo serializou o cold start com o lock existente, reservou IDs atomicamente e registrou SHA-256 do arquivo completo para rejeitar snapshot obsoleto. Esses ajustes sustentam a viabilidade, mas tambem impedem reutilizar a orquestracao original sem alteracoes.

## Limites

- A configuracao experimental usa ambiente, nao uma interface publica final.
- O `ConsumerID` reutiliza o slug de contexto local e nao teve colisoes de normalizacao esgotadas.
- O trace cobriu o apply representativo, nao todos os processos dos testes.
- O corpus recebeu IDs Markdown exigidos pelo candidato.
- Nao houve nova passagem de privacidade; a falha da Etapa 5 continua valida.

## Consequencia

O nucleo de operacoes Markdown, FTS5, staging/diff, lock e proveniencia segue para a Etapa 8 como candidato a reutilizacao de componente. Roots deixam de ser uma duvida arquitetural; privacidade, sessoes, integridade do staging/apply e isolamento de commits permanecem gates antes de qualquer incorporacao.
