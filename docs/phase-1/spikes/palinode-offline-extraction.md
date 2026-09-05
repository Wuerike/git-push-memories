# Spike: extracao offline do Palinode

## Decisao

**Resultado binario: passa. Disposicao revisada: candidato a componente.**

Na revisao fixa `9832d58663077645e93093bac9208a084b1d7492`, uma fatia de parser, executor e primitivas de evidencia executou sem runner, save, store, API, indice, sessao, embedder, Ollama, LLM, daemon ou banco. A evidencia foi preservada em `.artifacts/phase-1/20260905T200005Z-palinode-stage7-evidence`, conforme o [manifesto](../evidence-manifest.md).

## Fatia e interface

Doze blobs, incluindo `LICENSE` e inicializadores de pacote, foram extraidos diretamente dos objetos Git. Os SHA-256 conferiram com a revisao (`manifests/source-object-verification.txt`). A fatia incluiu `op_parse.py`, `executor.py`, parser, parity, hashing, quote verification, claims e typed links.

Uma unica facade de filesystem expos parse, inspecao de evidencia, verificacao de anchor e aplicacao revisada. Suas 150 linhas nao comentadas adicionaram contencao de root/path/symlink, guarda de baseline, escrita atomica individual e relatorio de hashes (`adapter/facade.py`; `manifests/adapter-metrics.txt`). O ambiente minimo continha somente `python-frontmatter==1.1.0` e `PyYAML==6.0.2`, ambos MIT.

## Ambiente e reproducao

A origem `https://github.com/phasespace-labs/palinode.git` estava limpa em `main` e foi clonada localmente para uma copia destacada na revisao fixa. O run usou Linux x86_64, Git 2.54.0, Python 3.11.9 para a extracao, Python 3.13.3 para a venv upstream resolvida pelo lock, uv 0.6.17, Bubblewrap 0.6.1 e strace 5.16. Preparacao e instalacao ocorreram antes da classificacao; testes, paridade, replay e probes executaram offline com ambiente vazio.

Preparacao e extracao usaram estes comandos:

```bash
env -i PATH="$PATH" HOME="$TOOL_HOME" GIT_CONFIG_GLOBAL=/dev/null \
  GIT_CONFIG_NOSYSTEM=1 git clone --local --no-hardlinks --no-checkout \
  /home/Wuerike.Cavalheiro/workspace/palinode "$CANDIDATE_ROOT"
env -i PATH="$PATH" HOME="$TOOL_HOME" GIT_CONFIG_GLOBAL=/dev/null \
  GIT_CONFIG_NOSYSTEM=1 git -C "$CANDIDATE_ROOT" checkout --detach \
  9832d58663077645e93093bac9208a084b1d7492
git -C "$CANDIDATE_ROOT" archive 9832d58663077645e93093bac9208a084b1d7492 \
  LICENSE palinode/__init__.py palinode/core/__init__.py \
  palinode/core/parser.py palinode/core/parity.py palinode/core/hashing.py \
  palinode/core/quote_verify.py palinode/core/claims.py \
  palinode/core/typed_links.py palinode/consolidation/__init__.py \
  palinode/consolidation/op_parse.py palinode/consolidation/executor.py \
  | tar -x -C "$EXTRACTED_ROOT"
uv venv --python 3.11 "$EXTRACTED_ROOT/.venv"
uv pip install --python "$EXTRACTED_ROOT/.venv/bin/python" \
  python-frontmatter==1.1.0 PyYAML==6.0.2
uv sync --extra dev
```

Os fluxos extraidos usaram dois wrappers offline. `run_extracted` executou os testes com a fatia gravavel; `run_harness` deixou fonte e harness somente leitura e recebeu apenas um output gravavel:

```bash
run_extracted() {
  bwrap --unshare-all --die-with-parent --new-session \
    --ro-bind /usr /usr --ro-bind /bin /bin --ro-bind /lib /lib \
    --ro-bind /lib64 /lib64 --ro-bind "$PYTHON311" /opt/python \
    --proc /proc --dev /dev --tmpfs /tmp \
    --dir /home --bind "$TOOL_HOME" /home/phase-1 \
    --dir /workspace --bind "$EXTRACTED_ROOT" /workspace/extracted \
    --chdir /workspace/extracted --clearenv \
    --setenv HOME /home/phase-1 --setenv PATH /opt/python/bin:/usr/bin:/bin \
    --setenv LD_LIBRARY_PATH /opt/python/lib \
    --setenv PYTHONPATH /workspace/extracted/.venv/lib/python3.11/site-packages:/workspace/extracted \
    --setenv LANG C.UTF-8 --setenv TZ UTC \
    --setenv TMPDIR /home/phase-1/tmp -- "$@"
}

run_harness() {
  local output=$1
  shift
  bwrap --unshare-all --die-with-parent --new-session \
    --ro-bind /usr /usr --ro-bind /bin /bin --ro-bind /lib /lib \
    --ro-bind /lib64 /lib64 --ro-bind "$PYTHON311" /opt/python \
    --proc /proc --dev /dev --tmpfs /tmp \
    --dir /home --bind "$TOOL_HOME" /home/phase-1 \
    --dir /workspace --ro-bind "$EXTRACTED_ROOT" /workspace/source \
    --ro-bind "$HARNESS_ROOT" /workspace/harness --bind "$output" /workspace/output \
    --chdir /workspace/harness --clearenv \
    --setenv HOME /home/phase-1 --setenv PATH /opt/python/bin:/usr/bin:/bin \
    --setenv LD_LIBRARY_PATH /opt/python/lib \
    --setenv PYTHONPATH /workspace/source/.venv/lib/python3.11/site-packages:/workspace/source \
    --setenv LANG C.UTF-8 --setenv TZ UTC \
    --setenv TMPDIR /home/phase-1/tmp -- "$@"
}
```

O replay usou o mesmo `run_harness`, acrescentando `--bind "$MEMORY_ROOT" /workspace/memory`, `--ro-bind "$CODE_ROOT" /workspace/code` e as variaveis `MEMORY_ROOT=/workspace/memory` e `CODE_ROOT=/workspace/code`. A suite upstream usou o helper do protocolo com Python 3.13.3, a copia candidata gravavel e ambiente vazio. Os comandos classificatorios centrais foram:

```bash
python3.13 -m pytest -q \
  tests/test_executor.py tests/test_executor_ops_coverage.py \
  tests/test_executor_replace_guard.py tests/test_op_parse.py \
  tests/test_epistemic_markers.py tests/test_typed_links.py \
  tests/test_quote_verify.py tests/test_quote_hash_prefixing.py \
  tests/test_claims_508.py tests/test_parity_is_dependency_free.py

python3.11 -m unittest -v test_facade
python3.11 -m unittest -v \
  test_facade.FacadeTests.test_facade_needs_no_network_or_child_process
python3.11 import_probe.py
python3.11 parity_driver.py /workspace/output
python3.11 checkout_replay.py \
  /workspace/memory /workspace/harness/checkout-proposal.json
```

A primeira tentativa upstream retornou 2 em 2,72 s por usar Python 3.11 com a venv resolvida para 3.13; foi preservada e substituida pela execucao correta.

| Comando | Codigo | Duracao |
|---|---:|---:|
| suite upstream corrigida | 0 | 16,40 s |
| matriz extraida | 0 | 0,59 s |
| addendum de isolamento | 0 | 0,29 s |
| import probe | 0 | 0,21 s |
| paridade upstream | 0 | 0,75 s |
| paridade extraida | 0 | 0,43 s |
| repeticao extraida | 0 | 0,36 s |
| replay do checkout | 0 | 0,31 s |

Saidas e tempos estao em arquivos homonimos. Os argumentos Bubblewrap expandidos e traces sanitizados estao em `sanitized-traces/`. `manifests/source-revision-status.txt`, `upstream-test-state-inventory.txt`, `memory-*`, `code-final-*`, hashes e diffs do checkout registram o estado antes e depois; as origens permaneceram limpas.

## Resultados

| Verificacao | Resultado |
|---|---|
| testes upstream corrigidos | 243 passam, 1 skip, 3 xfails esperados |
| matriz extraida | 9 passam |
| addendum de isolamento | passa |
| paridade com relogio fixo | saida byte a byte identica |
| repeticao da extracao | saida byte a byte identica |
| replay do corpus comum | dois `UPDATE`; tres segundos e `team-b` |
| segundo replay | bytes estaveis; dois `unmatched` reportados pelo upstream |
| importacoes proibidas carregadas | nenhuma |
| conexoes externas ou processos filhos | nenhum no fluxo extraido |

A paridade cobriu `KEEP`, `UPDATE`, `MERGE`, `SUPERSEDE`, `ARCHIVE`, `RETRACT`, `PROPOSE_CONTRADICTS`, guards e ordenacao (`parity-comparison.txt`; `parity-*.stdout`). A matriz tambem cobriu marcadores epistemicos, links tipados, quotes, claims, baseline divergente, paths absolutos, traversal, symlinks e injecao de falha (`extracted-tests.stderr`; `extracted-isolation-addendum.stderr`).

## Limites

- O corpus comum nao possui fact IDs; o harness adicionou dois comentarios antes do replay sem copiar bootstrap, runner ou save.
- JSON malformado retorna nenhuma operacao porque `json_repair` ficou fora da extracao.
- `UPDATE` nao transfere claims, sources ou linhagem de evidencia para o alvo atualizado.
- Escritas de history e arquivo principal sao atomicas individualmente, nao uma transacao conjunta.
- A facade usa contexto global serializado e nao serve como runtime concorrente multi-root.
- `typed_links.py` conserva um import lazy de indexer em funcao fora da facade; o probe confirmou que ele nao foi carregado.

## Licenca

A fatia esta coberta pela MIT raiz do Palinode. `python-frontmatter` e PyYAML tambem foram inventariados como MIT. Qualquer incorporacao deve preservar os avisos e excluir plugin, prompts, vetores CC BY, modelos e ativos que nao fizeram parte deste spike.

## Consequencia

Parser/executor offline e primitivas de quote, claim e typed links seguem para a Etapa 8 como candidatos provisorios a reutilizacao de componente. O produto completo continua inadequado como base, e a arquitetura consumidora deve fornecer proposta duravel, composicao de proveniencia, transacao/publicacao e concorrencia. Incorporacao depende do inventario exato da Etapa 8 e do ADR da Etapa 9.
