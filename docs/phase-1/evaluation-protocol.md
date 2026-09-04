# Protocolo de avaliacao da Fase 1

Este protocolo fixa o corpus, o ambiente, a coleta de evidencias e a escala usados para comparar os candidatos da Fase 1. Uma avaliacao pode acrescentar testes especificos de um projeto, mas nao pode alterar o significado dos criterios comuns.

## Escopo

O protocolo avalia se cada candidato pode servir como base, fornecer um componente isolado ou apenas informar uma reimplementacao. Ele nao autoriza incorporar codigo de candidatos ao `git-push-memories`.

Todas as execucoes usam dados sinteticos, clones locais nas revisoes fixadas e repositorios descartaveis. Nenhuma credencial, sessao ou memoria real pode ser usada.

## Pontos fixos

Revisoes confirmadas em 2026-09-04. Todos os clones estavam na branch `main`, alinhados com `origin/main` e sem alteracoes locais.

| Projeto | Repositorio | Clone local | Revisao |
|---|---|---|---|
| Basic Memory | `basicmachines-co/basic-memory` | `~/workspace/basic-memory` | `a55003e7d0ab36161fa5e933198e3f62907160e7` |
| ai-memory | `akitaonrails/ai-memory` | `~/workspace/ai-memory` | `3bd35154c845d5c2ab510e2c2ecae3c1d2059b19` |
| Palinode | `phasespace-labs/palinode` | `~/workspace/palinode` | `9832d58663077645e93093bac9208a084b1d7492` |
| claude-mem | `thedotmack/claude-mem` | `~/workspace/claude-mem` | `be44b6c8e238a7e2bc5b3403c05afac071a59ead` |
| claude-bedrock | `iurykrieger/claude-bedrock` | `~/workspace/claude-bedrock` | `7cac16ceb6064b7fbb4ec45c3ad4be31b9d9c9f0` |
| agent-memory | `xChuCx/agent-memory` | `~/workspace/agent-memory` | `51dac9bf6e72c67f6314ee3abb00515986a3fa4a` |
| twiceshy | `dotts-h/twiceshy` | `~/workspace/twiceshy` | `aa8f1d88b23ce26b9add249ee2f507d350aca12c` |

Antes de avaliar um projeto, confirmar o ponto fixo sem atualizar o clone:

```bash
git -C "$CANDIDATE_ROOT" rev-parse HEAD
git -C "$CANDIDATE_ROOT" branch --show-current
git -C "$CANDIDATE_ROOT" remote get-url origin
git -C "$CANDIDATE_ROOT" status --short --branch
```

Se a revisao ou a arvore diferir, interromper a execucao e registrar a divergencia. Nao usar `pull`, `fetch`, `reset`, `clean` nem qualquer comando que altere o clone durante a comparacao.

## Ambiente de referencia

Ambiente registrado em 2026-09-04:

| Item | Versao ou estado |
|---|---|
| Sistema | Linux 6.8.0-136-generic, x86_64 |
| Git | 2.54.0 |
| Claude Code | 2.1.260 |
| Node.js | 20.20.0 |
| npm | 10.8.2 |
| Bun | 1.3.11 |
| Python | 3.11.9 |
| uv | 0.6.17 |
| Go | 1.24.2 |
| Rust | 1.88.0-nightly (2025-04-24) |
| Docker | 29.7.2 |
| GNU grep | 3.7 |
| strace | 5.16 |
| Bubblewrap | 0.6.1 |
| ripgrep | nao instalado |
| sqlite3 CLI | nao instalado |

Cada relatorio deve registrar novamente apenas as ferramentas usadas. A ausencia do `sqlite3` CLI nao impede bibliotecas com SQLite embutido, mas qualquer inspecao que dependa do executavel deve instalar uma versao registrada ou usar uma interface fornecida pelo candidato.

As revisoes fixadas possuem requisitos superiores a alguns runtimes instalados:

| Projeto | Requisito declarado | Preparacao minima |
|---|---|---|
| Basic Memory | Python >=3.12 | provisionar Python 3.12 com `uv` |
| ai-memory | Rust 1.95 | provisionar o toolchain 1.95.0 com `rustup` |
| Palinode | Python >=3.11 | atendido pelo ambiente de referencia |
| claude-mem | Bun | atendido pelo ambiente de referencia |
| agent-memory | Go 1.25.0 para build a partir do fonte | usar Go 1.25.12 ou binario publicado da revisao, registrando a escolha |
| twiceshy | Go 1.25.12 | provisionar Go 1.25.12 |

Rede pode ser usada para instalar dependencias antes do teste, com comandos e lockfiles registrados. O fluxo usado para classificar operacao local deve ser repetido no sandbox sem rede depois que as dependencias estiverem disponiveis.

## Corpus comum

Os arquivos fonte ficam em [`corpus/`](corpus/):

- `memory-initial/services/checkout-api.md`: estado canonico inicial, com timeout de cinco segundos e ownership do `team-a`;
- `memory-expected/services/checkout-api.md`: estado final esperado, com timeout de tres segundos e ownership do `team-b`;
- `code-repo/services/checkout-api/service.yaml`: evidencia sintetica do estado novo;
- `code-repo/private-markers.txt`: marcadores sensiveis sinteticos usados no teste de privacidade.

Os marcadores fixos, definidos somente no fixture `private-markers.txt`, representam um segredo e um dado pessoal reconheciveis. Os valores literais nao devem ser repetidos em comandos, nomes de arquivo ou relatorios.

Esses valores nao sao segredos nem dados pessoais reais. Depois que o candidato recebe o conteudo, nenhuma ocorrencia nova pode existir na memoria, proposta, indice, cache, log ou estado proprio da ferramenta.

## Preparacao dos repositorios

Use um diretorio novo por projeto e por tentativa. Execute a partir da raiz do `git-push-memories`:

```bash
export RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)-<project>"
export RUN_ROOT="/tmp/git-push-memories-phase-1/$RUN_ID"
export MEMORY_ROOT="$RUN_ROOT/memory-repo"
export CODE_ROOT="$RUN_ROOT/code-repo"
export CANDIDATE_SOURCE="$HOME/workspace/<candidate>"
export CANDIDATE_ROOT="$RUN_ROOT/candidate"
export FULL_REVISION="<full-revision>"
export TOOL_HOME="$RUN_ROOT/tool-home"
export EVIDENCE_ROOT="$RUN_ROOT-evidence"
export HOST_UV="$(readlink -f "$(command -v uv)")"
export HOST_RUSTUP="$(readlink -f "$(command -v rustup)")"
export HOST_CLAUDE="$(readlink -f "$(command -v claude)")"
export HOST_BUN="$(readlink -f "$(command -v bun)")"
export HOST_NODE_ROOT="$(dirname "$(dirname "$(readlink -f "$(command -v node)")")")"
export HOST_GO_ROOT="$(dirname "$(dirname "$(readlink -f "$(command -v go)")")")"
umask 077
mkdir -p \
  "$MEMORY_ROOT/services" \
  "$CODE_ROOT/services/checkout-api" \
  "$TOOL_HOME/tmp" \
  "$TOOL_HOME/xdg-config" \
  "$TOOL_HOME/xdg-cache" \
  "$TOOL_HOME/xdg-data" \
  "$TOOL_HOME/xdg-state" \
  "$TOOL_HOME/xdg-runtime" \
  "$EVIDENCE_ROOT"

cp docs/phase-1/corpus/memory-initial/services/checkout-api.md \
  "$MEMORY_ROOT/services/checkout-api.md"
cp docs/phase-1/corpus/code-repo/services/checkout-api/service.yaml \
  "$CODE_ROOT/services/checkout-api/service.yaml"

git -C "$MEMORY_ROOT" init -b main
git -C "$MEMORY_ROOT" config user.name "Phase 1 Evaluator"
git -C "$MEMORY_ROOT" config user.email "phase-1@example.invalid"
git -C "$MEMORY_ROOT" add services/checkout-api.md
git -C "$MEMORY_ROOT" commit -m "seed initial memory"

git -C "$CODE_ROOT" init -b main
git -C "$CODE_ROOT" config user.name "Phase 1 Evaluator"
git -C "$CODE_ROOT" config user.email "phase-1@example.invalid"
git -C "$CODE_ROOT" add services/checkout-api/service.yaml
git -C "$CODE_ROOT" commit -m "seed synthetic code repository"

env -i PATH="$PATH" HOME="$TOOL_HOME" GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 \
  git clone --local --no-hardlinks --no-checkout "$CANDIDATE_SOURCE" "$CANDIDATE_ROOT"
env -i PATH="$PATH" HOME="$TOOL_HOME" GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 \
  git -C "$CANDIDATE_ROOT" checkout --detach "$FULL_REVISION"
git -C "$MEMORY_ROOT" rev-parse HEAD > "$EVIDENCE_ROOT/memory-baseline-commit.txt"
git -C "$CODE_ROOT" rev-parse HEAD > "$EVIDENCE_ROOT/code-baseline-commit.txt"
git -C "$CODE_ROOT" cat-file --batch-all-objects --batch-check='%(objectname)' \
  | sort -u > "$EVIDENCE_ROOT/code-baseline-objects.txt"

run_sandbox() {
  local network="$1"
  shift
  local -a network_args=()
  local -a trace_args=()

  if [[ "$network" == "networked" ]]; then
    network_args+=(--share-net)
  fi
  if [[ "${TRACE_WRITES:-0}" == "1" ]]; then
    trace_args+=(
      strace -ff -s 65535 -yy
      -e trace=%file,write,writev,pwrite64,pwritev,truncate,ftruncate,mmap,msync,copy_file_range,sendfile,splice,tee,vmsplice
      -o "$EVIDENCE_ROOT/${TRACE_NAME:-privacy}.trace"
    )
  fi

  "${trace_args[@]}" bwrap \
    --unshare-all \
    "${network_args[@]}" \
    --die-with-parent \
    --new-session \
    --ro-bind /usr /usr \
    --ro-bind /bin /bin \
    --ro-bind /lib /lib \
    --ro-bind /lib64 /lib64 \
    --dir /etc \
    --ro-bind /etc/ssl /etc/ssl \
    --ro-bind /etc/ca-certificates /etc/ca-certificates \
    --ro-bind-try /etc/resolv.conf /etc/resolv.conf \
    --ro-bind-try /etc/hosts /etc/hosts \
    --ro-bind-try /etc/nsswitch.conf /etc/nsswitch.conf \
    --proc /proc \
    --dev /dev \
    --tmpfs /tmp \
    --dir /opt \
    --dir /opt/host-bin \
    --ro-bind "$HOST_UV" /opt/host-bin/uv \
    --ro-bind "$HOST_RUSTUP" /opt/host-bin/rustup \
    --ro-bind "$HOST_RUSTUP" /opt/host-bin/cargo \
    --ro-bind "$HOST_RUSTUP" /opt/host-bin/rustc \
    --ro-bind "$HOST_RUSTUP" /opt/host-bin/rustdoc \
    --ro-bind "$HOST_CLAUDE" /opt/host-bin/claude \
    --ro-bind "$HOST_BUN" /opt/host-bin/bun \
    --ro-bind "$HOST_NODE_ROOT" /opt/node \
    --ro-bind "$HOST_GO_ROOT" /opt/go \
    --dir /home \
    --bind "$TOOL_HOME" /home/phase-1 \
    --dir /workspace \
    --bind "$MEMORY_ROOT" /workspace/memory \
    --bind "$CODE_ROOT" /workspace/code \
    --bind "$CANDIDATE_ROOT" /workspace/candidate \
    --chdir /workspace/candidate \
    --clearenv \
    --setenv HOME /home/phase-1 \
    --setenv USER phase-1 \
    --setenv LOGNAME phase-1 \
    --setenv PATH /home/phase-1/cargo/bin:/home/phase-1/go/bin:/opt/node/bin:/opt/go/bin:/opt/host-bin:/usr/bin:/bin \
    --setenv LANG C.UTF-8 \
    --setenv TZ UTC \
    --setenv TMPDIR /home/phase-1/tmp \
    --setenv XDG_CONFIG_HOME /home/phase-1/xdg-config \
    --setenv XDG_CACHE_HOME /home/phase-1/xdg-cache \
    --setenv XDG_DATA_HOME /home/phase-1/xdg-data \
    --setenv XDG_STATE_HOME /home/phase-1/xdg-state \
    --setenv XDG_RUNTIME_DIR /home/phase-1/xdg-runtime \
    --setenv CLAUDE_CONFIG_DIR /home/phase-1/claude \
    --setenv NPM_CONFIG_CACHE /home/phase-1/npm-cache \
    --setenv BUN_INSTALL /home/phase-1/bun \
    --setenv UV_CACHE_DIR /home/phase-1/uv-cache \
    --setenv UV_PYTHON_INSTALL_DIR /home/phase-1/uv-python \
    --setenv CARGO_HOME /home/phase-1/cargo \
    --setenv RUSTUP_HOME /home/phase-1/rustup \
    --setenv GOPATH /home/phase-1/go \
    --setenv GOMODCACHE /home/phase-1/go/pkg/mod \
    --setenv GOCACHE /home/phase-1/go-build \
    --setenv GOENV off \
    --setenv GIT_CONFIG_GLOBAL /dev/null \
    --setenv GIT_CONFIG_NOSYSTEM 1 \
    --setenv MEMORY_ROOT /workspace/memory \
    --setenv CODE_ROOT /workspace/code \
    -- "$@"
}

run_isolated() { run_sandbox offline "$@"; }
run_networked() { run_sandbox networked "$@"; }
```

Nao reutilize um `RUN_ROOT`. Execute instalacao e download por `run_networked`; execute build, teste e todo fluxo avaliado por `run_isolated`. O Bubblewrap expoe apenas sistema e ferramentas como leitura, alem dos tres diretorios descartaveis como escrita. O ambiente vazio impede heranca acidental de tokens, proxies, agentes SSH e configuracoes reais. `EVIDENCE_ROOT` nao e visivel dentro do sandbox.

O socket Docker do host nao e montado e nao pode ser usado como evidencia: ele permitiria escrita e processos fora do inventario. Se um candidato exigir containers, use um engine rootless iniciado dentro do sandbox, com todo estado em `TOOL_HOME`, ou registre o fluxo como bloqueado. Se o Bubblewrap ou o isolamento de rede nao funcionar no ambiente, interrompa a execucao; apenas limpar variaveis de ambiente nao satisfaz este protocolo.

Nao repasse nenhuma variavel adicional sem inspecionar e registrar seu valor sanitizado. Nunca use autenticacao real; se uma integracao exigir credencial, use apenas credencial sintetica de um servico descartavel ou registre o passo como bloqueado.

O clone dentro de `CANDIDATE_ROOT` e a copia executavel descartavel. `CANDIDATE_SOURCE` serve somente como origem local e nao deve ser executado nem alterado. Antes de continuar, confirme que a revisao destacada e a esperada e que os tres repositorios estao limpos.

Depois de definir os helpers, provisione os toolchains requeridos. Exemplos:

```bash
run_networked uv python install 3.12
run_networked rustup toolchain install 1.95.0 --profile minimal
run_isolated rustup default 1.95.0
run_networked env GOTOOLCHAIN=go1.25.12 go version
```

## Cenario comum

Quando a interface do candidato permitir, executar nesta ordem:

1. Registrar o baseline dos dois repositorios e dos diretorios de dados da ferramenta.
2. Ingerir ou indexar apenas a memoria inicial.
3. Consultar `Qual e o timeout de checkout-api e qual time e responsavel?`.
4. Apresentar a descoberta `checkout-api usa timeout de tres segundos e agora pertence ao team-b`, sem os marcadores sensiveis, no run comportamental.
5. Confirmar que a descoberta produz uma proposta inspecionavel sem alterar o Markdown canonico.
6. Ajustar ou rejeitar a proposta, quando suportado, e confirmar que a memoria permanece intacta.
7. Gerar novamente e aceitar explicitamente uma proposta valida.
8. Confirmar que o Markdown representa apenas tres segundos e `team-b` como estado autoritativo.
9. Repetir a aplicacao e verificar idempotencia.
10. Consultar a informacao em uma sessao ou processo novo, sem acesso ao contexto da descoberta.
11. Remover somente o indice derivado em uma copia descartavel, reconstrui-lo a partir do Markdown e repetir a consulta.
12. Em um run novo, repetir a descoberta junto dos marcadores pela [passagem de privacidade](#passagem-de-privacidade).
13. Nesse run de privacidade, procurar os marcadores fora do arquivo fonte permitido e inventariar tudo que foi persistido.

Se uma interface nao suportar um passo, registrar `nao suportado` em vez de simular a capacidade por alteracao manual. Alteracoes manuais do Markdown sao permitidas apenas nos testes que explicitamente avaliam deteccao e reindexacao de mudancas externas.

## Coleta de evidencias

Cada projeto tera um relatorio em `docs/phase-1/evaluations/<project>.md`. Comandos, saidas pequenas e extratos sanitizados ficam no relatorio. Artefatos brutos ficam em `EVIDENCE_ROOT`, fora de `RUN_ROOT`, para nao serem confundidos com persistencia do candidato. Antes de remover o ambiente, transcrever para o relatorio toda evidencia necessaria para reproduzir a classificacao.

Preservar no minimo:

- data, hipotese, escopo e exclusoes;
- revisao completa, branch, remoto e estado inicial do clone;
- sistema, runtimes, gerenciadores e dependencias efetivamente usados;
- configuracao efetiva, com qualquer valor nao sintetico removido;
- comando literal, codigo de saida e duracao;
- inventario dos arquivos antes e depois;
- `git status`, hashes e diff do Markdown canonico;
- consulta, resultado, latencia e tamanho aproximado da resposta;
- inventario de bancos, tabelas, caches e logs;
- busca pelos dois marcadores sinteticos;
- testes relevantes e classificacao ligada a uma evidencia.

Comandos comuns de coleta:

```bash
date --iso-8601=seconds
git -C "$MEMORY_ROOT" status --short --branch
git -C "$MEMORY_ROOT" ls-files --stage
git -C "$MEMORY_ROOT" diff --no-ext-diff --binary HEAD --
git -C "$MEMORY_ROOT" diff --no-ext-diff --binary --cached
git -C "$MEMORY_ROOT" hash-object services/checkout-api.md
find "$RUN_ROOT" -path '*/.git' -prune -o -type f -printf '%P\t%s bytes\n' | sort
grep -E '^(secret_marker|personal_marker)=' \
  docs/phase-1/corpus/code-repo/private-markers.txt \
  | cut -d= -f2- \
  > "$EVIDENCE_ROOT/marker-patterns.txt"
grep -R -n -F -f "$EVIDENCE_ROOT/marker-patterns.txt" \
  --binary-files=text \
  --exclude-dir=.git \
  "$MEMORY_ROOT" "$CODE_ROOT" "$CANDIDATE_ROOT" "$TOOL_HOME" \
  > "$EVIDENCE_ROOT/marker-hits.txt"
git -C "$MEMORY_ROOT" grep --cached -n -F \
  -f "$EVIDENCE_ROOT/marker-patterns.txt" \
  > "$EVIDENCE_ROOT/memory-index-marker-hits.txt"
git -C "$MEMORY_ROOT" grep -n -F \
  -f "$EVIDENCE_ROOT/marker-patterns.txt" \
  $(git -C "$MEMORY_ROOT" rev-list --all) \
  > "$EVIDENCE_ROOT/memory-history-marker-hits.txt"
grep -R -n -F -f "$EVIDENCE_ROOT/marker-patterns.txt" \
  --binary-files=text \
  --exclude-dir=objects \
  "$MEMORY_ROOT/.git" "$CANDIDATE_ROOT/.git" \
  > "$EVIDENCE_ROOT/git-control-marker-hits.txt"
grep -R -n -F -f "$EVIDENCE_ROOT/marker-patterns.txt" \
  --binary-files=text \
  --exclude-dir=objects \
  --exclude=index \
  "$CODE_ROOT/.git" \
  > "$EVIDENCE_ROOT/code-git-control-marker-hits.txt"
git -C "$MEMORY_ROOT" cat-file --batch-all-objects --batch \
  | grep -a -n -F -f "$EVIDENCE_ROOT/marker-patterns.txt" \
  > "$EVIDENCE_ROOT/memory-object-marker-hits.txt"
git -C "$CANDIDATE_ROOT" cat-file --batch-all-objects --batch \
  | grep -a -n -F -f "$EVIDENCE_ROOT/marker-patterns.txt" \
  > "$EVIDENCE_ROOT/candidate-object-marker-hits.txt"
git -C "$CODE_ROOT" grep --cached -n -F \
  -f "$EVIDENCE_ROOT/marker-patterns.txt" \
  > "$EVIDENCE_ROOT/code-index-marker-hits.txt"
git -C "$CODE_ROOT" grep -n -F \
  -f "$EVIDENCE_ROOT/marker-patterns.txt" \
  $(git -C "$CODE_ROOT" rev-list --all) \
  > "$EVIDENCE_ROOT/code-history-marker-hits.txt"
git -C "$CODE_ROOT" cat-file --batch-all-objects --batch-check='%(objectname)' \
  | sort -u > "$EVIDENCE_ROOT/code-final-objects.txt"
comm -13 \
  "$EVIDENCE_ROOT/code-baseline-objects.txt" \
  "$EVIDENCE_ROOT/code-final-objects.txt" \
  | git -C "$CODE_ROOT" cat-file --batch \
  | grep -a -n -F -f "$EVIDENCE_ROOT/marker-patterns.txt" \
  > "$EVIDENCE_ROOT/code-new-object-marker-hits.txt"
```

Execute os scans de marcadores somente no run de privacidade. Se o candidato fizer commit, compare tambem o estado final com o commit inicial registrado em E1; `git diff HEAD` sozinho nao cobre esse caso. Na busca final, os unicos hits esperados sao as duas linhas do fixture em `CODE_ROOT/private-markers.txt`. Nenhum indice, historico, arquivo de controle ou objeto Git novo deve ter hits. Como o baseline e criado sem os marcadores, a busca nos objetos novos tambem detecta blobs soltos e arvores que os referenciem. Indice, arquivos de controle e todos os objetos alcancaveis ou soltos dos outros repositorios tambem nao devem ter hits. Inspecione da mesma forma todo repositorio Git adicional encontrado no inventario. O proprio arquivo de padroes e a saida da busca ficam fora do estado inspecionado e devem ser sanitizados antes de entrar no relatorio.

Para cada fluxo principal, use `/usr/bin/time` ou mecanismo equivalente e registre comando, saida padrao, erro padrao, codigo de saida e tempo decorrido separadamente. Nao preserve transcricoes completas, prompts reais, respostas reais nem referencias de sessao. Se o candidato os persistir, registre caminho, tipo e tamanho, remova o conteudo do extrato e classifique o comportamento.

## Passagem de privacidade

A busca final prova apenas o estado ao termino. Para avaliar a fronteira antes da primeira persistencia, crie um `RUN_ID` novo e repita preparacao, provisionamento, build, ingestao da memoria inicial e consulta sem copiar nem expor `private-markers.txt`. Nenhum processo do candidato pode acessar a raiz do `git-push-memories`, e o fixture nao existe nos diretorios montados nesse ponto.

Depois que toda preparacao do candidato terminar, copie o fixture pelo host e inicie imediatamente o primeiro comando que pode le-lo sob trace:

```bash
cp docs/phase-1/corpus/code-repo/private-markers.txt "$CODE_ROOT/private-markers.txt"
TRACE_WRITES=1 TRACE_NAME=privacy run_isolated \
  <candidate-command-that-first-receives-private-markers>
```

Nao execute nenhum comando do candidato entre o `cp` e o comando rastreado. O arquivo permanece como a unica ocorrencia permitida no estado inspecionado e nao e adicionado ao Git. A primeira exposicao ao candidato acontece, portanto, depois que `strace` iniciou e durante o fluxo rastreado.

Se a ferramenta usa daemon, execute um unico script pelo helper: ele deve iniciar uma instancia nova como processo filho, aguardar prontidao, executar o cliente que envia os marcadores e encerrar o daemon. Assim, daemon e cliente compartilham o mesmo sandbox e ficam cobertos pelo `strace -ff`. Nao conecte o teste a daemon iniciado anteriormente. Se o processo se desprender ou a cobertura de todos os escritores nao puder ser demonstrada, classifique o criterio como `parcial`.

Inspecione os traces usando o arquivo de padroes, relacione descritores aos caminhos abertos e registre somente extratos sanitizados. Uma escrita dos marcadores em arquivo, banco, log, cache ou memoria mapeada falha o criterio mesmo que seja renomeada ou removida depois. Escrita somente em `stdout` ou `stderr` nao prova persistencia pelo candidato, mas deve ser registrada como exposicao e nunca redirecionada sem sanitizacao.

`strace` nao demonstra o conteudo de toda escrita por `mmap` nem garante cobertura fora de Linux. Quando o caminho de persistencia nao puder ser observado completamente, combine trace, leitura do codigo e teste focado da fronteira; sem essa evidencia, classifique como `parcial`, nunca `passa`.

## Escala de classificacao

| Valor | Regra |
|---|---|
| `passa` | A capacidade atende integralmente ao criterio comum e ha evidencia reproduzivel. |
| `parcial` | A capacidade existe, mas depende de configuracao, adaptacao ou garantia incompleta claramente registrada. |
| `falha` | O comportamento observado contradiz o criterio ou a arquitetura torna seu atendimento inviavel sem substituir parte central. |
| `não se aplica` | O criterio esta fora do papel avaliado para o candidato e a justificativa esta registrada. Nao significa sucesso. |
| `pendente` | Ainda nao ha evidencia suficiente. Valor permitido somente enquanto a avaliacao estiver aberta. |

Uma classificacao final nunca pode permanecer `pendente`. Documentacao ou leitura de codigo podem bastar para criterios estruturais; criterios comportamentais exigem execucao, salvo quando a ausencia da capacidade estiver inequivocamente demonstrada.

## Regras por criterio

| Criterio | Condicao minima para `passa` |
|---|---|
| Markdown e Git canonicos | Remover ferramenta e indice nao elimina nem torna ilegivel a memoria. |
| Operacao local | O fluxo principal conclui sem servico remoto obrigatorio. |
| Ausencia de backend obrigatorio | Nenhum daemon ou infraestrutura adicional precisa permanecer ativo no fluxo minimo. |
| Indice reconstruivel | Excluir e reconstruir o indice somente do Markdown preserva a resposta correta. |
| Privacidade antes da persistencia | Trace e teste focado demonstram descarte antes da primeira escrita; a busca final nao encontra ocorrencias novas. |
| Sem persistencia de sessoes | Nenhum prompt, resposta, transcricao ou identificador de sessao e armazenado. |
| Estado atual | O resultado final apresenta tres segundos e `team-b`, sem o estado antigo como igualmente autoritativo. |
| Consolidacao antes da publicacao | Existe representacao temporaria observavel antes da alteracao canonica. |
| Revisao explicita | A pessoa pode inspecionar, ajustar, rejeitar e aceitar antes da aplicacao. |
| Repositorio de memoria separado | `MEMORY_ROOT` e `CODE_ROOT` podem ser diferentes, e a memoria pode referenciar mais de um repositorio de codigo. |
| Integracao com Claude Code | Captura e recuperacao participam do ciclo de vida do Claude Code em configuracao isolada. |
| Testabilidade | Contratos criticos possuem testes deterministas executaveis na revisao fixada. |
| Licenca compativel | A forma de adocao proposta preserva Apache-2.0 e todas as obrigacoes foram identificadas. |
| Custo de adaptacao | Mudancas necessarias, partes substituidas e dependencias operacionais estao delimitadas qualitativamente. |

## Relatorio e conclusao

Copie [`evaluations/template.md`](evaluations/template.md) para iniciar uma avaliacao. Toda classificacao deve apontar para uma secao de evidencia no mesmo relatorio. A disposicao recomendada deve ser uma entre `fork`, `componente`, `reimplementacao conceitual`, `referencia` ou `rejeicao`.

Uma avaliacao esta concluida quando nao possui criterios `pendente`, explica passos nao executados, delimita duvidas restantes e declara se essas duvidas podem alterar a decisao. Somente duvidas decisivas podem originar um spike.
