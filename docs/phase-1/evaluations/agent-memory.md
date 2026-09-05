# Avaliacao de agent-memory

## Identificacao

| Campo | Valor |
|---|---|
| Projeto | agent-memory |
| Repositorio | `https://github.com/xChuCx/agent-memory.git` |
| Clone local | `~/workspace/agent-memory` |
| Revisao | `51dac9bf6e72c67f6314ee3abb00515986a3fa4a` |
| Branch | `main`, alinhada a `origin/main` |
| Estado inicial | arvore limpa |
| Data | 2026-09-04 (triagem estatica); 2026-09-05 (Etapas 5, 6 e 7) |
| Avaliacao | triagem estatica, governanca offline, integracao com Claude Code e spike de roots |
| Avaliador | OpenCode |
| Run IDs | `20260905T024115Z-agent-memory-stage5`; `stage6-agent-memory-run`; `20260905T170000Z-agent-memory-stage7` |
| Commit inicial da memoria | lifecycle `ba3db8ff96dd79f334108e12b7206655c4dc3bd6`; demais fixtures em `*-baseline-commit.txt` e `privacy-*/prep/baseline-commit.txt` |

## Hipotese

agent-memory pode fornecer o nucleo de operacoes, diff e aplicacao explicita, desde que memoria e codigo possam ter roots distintos e as lacunas de privacidade e drift sejam corrigiveis sem reescrever o componente.

## Escopo e metodo

Na triagem estatica foram inspecionados CLI, MCP, schema, update/staging, scanner, indice, Git, adaptador Claude e testes. A Etapa 5 executou os fluxos de governanca descritos abaixo sem alterar o clone original. A Etapa 6 carregou a skill e o servidor MCP em Claude Code e exercitou proposta e recuperacao por processos MCP novos. A Etapa 7 usou um prototipo descartavel para separar roots e testar dois consumidores concorrentes.

```bash
git rev-parse HEAD
git branch --show-current
git remote get-url origin
git status --short --branch
git log -1 --format='%cI %s'
git rev-list --count HEAD
git rev-list --count --since='2026-08-05T00:00:00Z' HEAD
git shortlog -sne HEAD
```

Os comandos retornaram `0`; a duracao nao foi medida. O projeto usa Go 1.25, Cobra, MCP Go SDK, Goldmark, YAML, locks e SQLite/FTS5 puro Go (`go.mod:1-12`). O historico local tem 97 commits, nenhum desde 2026-08-05 e autoria efetivamente concentrada em uma identidade; a revisao/tag 0.5.1 e de 2026-06-09.

Ambiente usado na triagem: Linux x86_64 e Git 2.54.0, conforme [`evaluation-protocol.md`](../evaluation-protocol.md#ambiente-de-referencia). Somente Git e leitura estatica de arquivos foram usados.

## Fluxo estatico

| Estagio | Evidencia no clone | Conclusao |
|---|---|---|
| Entrada | `internal/cli/propose.go:59-153`; `internal/mcp/propose.go:92-157` | CLI/MCP recebem operacoes estruturadas. |
| Planejamento | `internal/memory/update.go:173-198,226-459` | Valida categoria/schema, aplica operacoes em memoria e gera post-state. |
| Scan | `internal/memory/secrets.go:73-180`; `internal/memory/pii.go:20-58`; `internal/memory/update.go:462-490` | Scan cobre formatos conhecidos e o Markdown final, nao todos os metadados. |
| Staging | `internal/memory/update.go:751-839` | Persiste request, checksums e copias integrais propostas. |
| Revisao | `internal/cli/review.go:104-157`; `internal/cli/diff.go:80-165` | Exibe diff; reject e apply sao comandos separados. |
| Aplicacao | `internal/memory/staging.go:215-314,410-496` | Verifica drift conforme politica e sobrescreve arquivos staged. |
| Recuperacao | `internal/memory/fetch.go:160-193`; `internal/index/query.go:40-72`; `internal/index/rebuild.go:28-48` | FTS5 e reconstruido do Markdown. |

## Armazenamento e roots

Markdown em `.agent-memory/` e canonico; staging, sessions e logs sao locais; SQLite e indice shadow (`agent-memory-design-doc-v0.4.1.md:292-330`; `internal/index/sqlite.go:20-35`). O rebuild pode inserir IDs no Markdown (`internal/index/rebuild.go:137-163`), portanto ainda requer teste de nao alteracao indevida.

O risco decisivo e o root: `--root` e raiz do projeto e a memoria e sempre `<root>/.agent-memory` (`internal/cli/helpers.go:13-30`). Branch/changed files e auto-stage usam o mesmo repositorio (`internal/cli/fetch.go:116-150`; `internal/memory/update.go:656-660`). `manifest.project.root` nao e consumido. Stores federados sao read-only (`internal/config/stores.go:26-29`). Logo, o requisito de repositorio de memoria separado falha sem uma mudanca transversal.

Nao ha backend remoto ou daemon permanente. MCP e stdio; SQLite e embutido (`internal/mcp/server.go:71-76`).

## Privacidade, sessoes e drift

Os marcadores sinteticos do protocolo nao correspondem aos prefixos secretos nem aos defaults de PII: o segredo fica abaixo do limiar de entropia e e-mail vem desabilitado. Apenas o post-state Markdown e escaneado; `rationale`, `sources`, owner e outros metadados entram em `proposal.json` sem scan (`internal/memory/update.go:811-827`). Lock info pode receber owner/op antes do scan (`internal/lock/lock.go:107-118`). `apply` nao reescaneia nem revalida staging (`internal/memory/staging.go:410-424`).

O layout persiste `sessions/*.md`, owner/session refs, propostas e sidecars, contrariando a politica comum de nao persistir material ou IDs de sessao.

Algumas operacoes usam precondicoes fracas, como arquivo presente ou secao resolvivel (`internal/memory/operations.go:475-481,548-553`). Como apply grava o snapshot integral, uma mudanca concorrente fora da secao pode ser perdida. Aplicacao multi-arquivo tambem nao e transacional.

## Estado atual, governanca e publicacao

`replace_section`, `replace_section_content`, archive e delete fornecem boas primitivas (`internal/memory/operations.go:347-419,570-626,755-913`), mas o agente precisa identificar a contradicao. Append pode manter dois estados autoritativos.

Review permite inspecao/diff, reject e apply. Nao ha comando seguro para ajustar. Editar staging manualmente e inseguro porque apply nao verifica o post-state nem repete scan/schema. Categorias duraveis normalmente stageiam, mas pitfalls podem auto-aplicar e `--apply` contorna revisao separada.

Git add usa caminhos explicitos, mas commit inclui qualquer alteracao previamente staged (`internal/git/commit.go:10-86`). Nao ha branch, push ou PR. O merge driver por IDs continua relevante (`internal/markdown/merge.go:8-30`).

## Claude Code e testes

O adaptador instala skill e `.mcp.json`; a skill instrui fetch no inicio e propose no fim (`internal/adapters/claude/SKILL.md:23-69`; `internal/adapters/claude/claude.go:71-181`). Nao ha hooks; o modelo precisa seguir a instrucao. A Etapa 6 confirmou que Claude Code carrega ambos, mas a ausencia deliberada de credencial impediu observar o modelo chamando as ferramentas.

Foram encontrados 83 arquivos de teste. A CI declara build/vet/test em tres sistemas, race, E2E e lint (`.github/workflows/ci.yml:8-86`). O E2E percorre init, MCP, propose, review, apply, fetch, segredo e archive (`internal/e2e/release01_smoke_test.go:30-68,103-621`). Ha testes de rebuild e drift (`internal/cli/rebuild_index_test.go:32-151`; `internal/memory/rebase_test.go:213-315`), mas nao cobrem roots separados, tampering, drift fora da secao ou staged alheio. A execucao da suite esta registrada na Etapa 5.

## Etapa 5: governanca

### Ambiente e reproducao

A evidencia valida foi preservada em `.artifacts/phase-1/20260905T024115Z-agent-memory-stage5-evidence`, conforme o [manifesto de evidencias](../evidence-manifest.md). O harness completo esta em `harness/`; para repetir, copie seus seis arquivos para `/tmp/opencode/` e execute `bash /tmp/opencode/run-agent-memory-stage5.sh`. A localizacao externa e intencional conforme o protocolo: artefatos brutos e scripts executaveis ficam no `EVIDENCE_ROOT`, enquanto comandos e conclusoes classificatorias sao transcritos neste relatorio. Os caminhos absolutos do runner correspondem aos clones fixos registrados no protocolo. O runner criou um clone descartavel destacado na revisao fixa, provisionou Go 1.25.12 com rede e depois compilou e executou todos os fluxos sem rede ou credenciais. O binario reportou `v0.5.1`; o build demorou 15,23 s e terminou com codigo zero. Clones original e descartavel permaneceram limpos.

Como `--root` une codigo e memoria, cada fixture usou o repositorio de codigo como root e adaptou o corpus comum para `.agent-memory/modules/checkout-api.md`, acrescentando apenas os IDs exigidos pelo schema. O baseline canonico continha cinco segundos e `team-a`; `services/checkout-api/service.yaml` forneceu a proveniencia para a operacao estruturada:

Os commits baseline foram: lifecycle `ba3db8ff96dd79f334108e12b7206655c4dc3bd6`; tamper e drift interno `39ed52dbdbd16354908f318cc3291232a0d3375f`; drift externo `fee99340820c8a5dc1104219bae19dcc39f49776`; Git default `ec7c0c03b18c9a8d9f5d75dfa798cf3483bc73c3`, stage `de0c7f5ed3ee877bcf80a7b2fce96b0626877fb6` e commit `270c268aa6dbe78a226acae2a2c664a8a4021854`; privacidade de corpo `85150acc1d7ce1fd442090add0303422123e3b81`, metadados `71c5148f9bd48e1c794d9988b2d1b63194c4fb80` e controle `66e94db5bcb05b6f7730a8f1da97426e3a985858`.

```bash
/home/phase-1/bin/agent-memory propose \
  --root /workspace/cases/lifecycle \
  --from-json /home/phase-1/tmp/main-artifacts/lifecycle-rejected-request.json --json
/home/phase-1/bin/agent-memory review \
  --root /workspace/cases/lifecycle \
  20260905T024248-refresh-module-stage5-reject-first-propo --show --diff --json
/home/phase-1/bin/agent-memory reject \
  --root /workspace/cases/lifecycle \
  20260905T024248-refresh-module-stage5-reject-first-propo --json
/home/phase-1/bin/agent-memory propose \
  --root /workspace/cases/lifecycle \
  --from-json /home/phase-1/tmp/main-artifacts/lifecycle-applied-request.json --json
/home/phase-1/bin/agent-memory apply \
  --root /workspace/cases/lifecycle \
  20260905T024250-refresh-module-stage5-regenerated-valid --json
```

O request usou `refresh_module` e `replace_section_content` na secao `checkout-api-operacao`, trocando o corpo por tres segundos e `team-b`. `propose` levou 0,04 s, `review` e `reject` menos de 0,01 s, a proposta regenerada 0,03 s e `apply` 0,02 s. Todos terminaram com codigo zero.

Tentativas anteriores nao sustentam classificacao: `20260905T021803Z-agent-memory-stage5` parou porque o harness esperava codigo nao zero de uma rejeicao JSON; `20260905T022042Z-agent-memory-stage5` falhou no download do toolchain por timeout TLS; `20260905T022208Z-agent-memory-stage5` e `20260905T022726Z-agent-memory-stage5` completaram, mas foram substituidas para acrescentar duracao, consulta, inventarios e scans completos das passagens de privacidade.

`Operacao local` abaixo se refere ao fluxo principal de governanca executado nesta etapa, que concluiu integralmente offline. Rebuild comparativo, recuperacao em processo novo e idempotencia pertencem as Etapas 3/4 ainda abertas para este candidato e continuam explicitamente listados como trabalho restante; o sucesso local nao os classifica por inferencia.

### Proposta, revisao e aplicacao

A proposta criou `proposal.json`, `target-checksums.json` e uma copia integral do post-state sob `staging/<id>/files/`. O hash da pagina canonica e HEAD permaneceram no baseline. `review --show --diff` exibiu o post-state completo, um unified diff com as quatro linhas de estado e a precondicao `require_section_content_match` com hash da secao.

`reject` retornou `rejected_by_user`, removeu o staging e nao alterou o canônico. Ele criou `meta/rejection-log.jsonl`, que o scaffold nao ignora e apareceu como untracked. Uma proposta nova foi revisada e aplicada explicitamente; a pagina final continha somente tres segundos e `team-b`, o staging foi removido, e Git continuou sem staging ou commit automatico na configuracao padrao. O indice Markdown gerado tambem mudou no worktree.

Nao existe comando de ajuste. O caminho seguro disponivel e rejeitar e gerar outro staging. `propose --apply` tambem permite contornar o comando separado de revisao. Portanto proposta, diff, rejeicao e aceite sao operacionais, mas o criterio completo de ajuste nao e atendido.

### Integridade e drift

Em uma fixture separada, uma proposta limpa passou pelo scan e depois teve o arquivo staged adulterado com um segredo sintetico reconhecido pelo scanner. `review --diff` mostrou o conteudo adulterado, mas `apply` terminou em 0,02 s com `status=applied`: nao houve hash do post-state, nova validacao de schema ou novo scan. O marcador foi encontrado na pagina canonica e duas vezes no SQLite. Assim, o staging e observavel, mas nao e um artefato integro ou seguro para edicao manual.

Uma mudanca concorrente dentro de `checkout-api-operacao` foi detectada pela divergencia do hash: `apply` retornou `status=rejected` e `reason=target_drift`, preservando o staging. `rebase` sem `--force` retornou `force_required`; com `--force`, recalculou proposta e checksum, ainda exigindo um segundo `apply` explicito. Entretanto, tanto `apply --json` quanto `rebase --json` encerraram com codigo zero nos resultados rejeitados, porque o renderer JSON retorna antes da verificacao de status (`internal/cli/apply.go:47-60`; `internal/cli/rebase.go:60-73`).

```bash
/home/phase-1/bin/agent-memory apply --root /workspace/cases/drift-inside \
  20260905T024250-refresh-module-stage5-drift-inside --json
/home/phase-1/bin/agent-memory rebase --root /workspace/cases/drift-inside \
  20260905T024250-refresh-module-stage5-drift-inside --json
/home/phase-1/bin/agent-memory rebase --root /workspace/cases/drift-inside \
  20260905T024250-refresh-module-stage5-drift-inside --force --json
```

Outra fixture acrescentou uma secao concorrente valida fora da secao protegida. O hash alvo continuou igual, `apply` retornou `applied` em 0,02 s e a copia integral staged removeu silenciosamente a nova secao. A precondicao por secao, portanto, nao protege o restante do mesmo arquivo contra overwrite.

### Privacidade

Tres fixtures novas foram preparadas, indexadas e consultadas antes da exposicao. A consulta literal `agent-memory fetch --root /workspace/case 'checkout-api timeout ownership'` levou 0,02 s, produziu 248 bytes e recuperou cinco segundos e `team-a`. Antes do indice havia 12 arquivos; depois da preparacao havia 14, incluindo `meta/index.sqlite` de 40.960 bytes e `meta/lock.info`. O SQLite tinha oito tabelas, cinco documentos e seis secoes. Cada passagem foi iniciada imediatamente apos a copia do fixture permitido e coberta por `strace -ff`; os relatorios preservam apenas nomes e contagens dos marcadores.

| Passagem | Duracao | Resultado | Persistencia observada |
|---|---:|---|---|
| marcadores fixos no corpo | 6,31 s | proposta staged | ambos em `proposal.json` e na copia integral staged |
| marcadores fixos em metadados | 6,24 s | proposta staged | ambos em `proposal.json`; segredo tambem em `meta/lock.info` |
| segredo reconhecido, controle positivo | 0,39 s | `secret_detected` | nenhuma ocorrencia no estado da fixture |

O controle positivo confirma que o scanner rejeita um formato conhecido antes do staging e redige a saida. Para os marcadores fixos, a busca cobriu fixture, clone do candidato, tool home e os bancos de objetos Git; os unicos hits alem do arquivo fonte permitido foram os artefatos do proprio candidato descritos na tabela. A passagem de corpo terminou com 18 arquivos, tres deles sob staging; SQLite continuou com as mesmas oito tabelas e contagens porque a proposta nao foi aplicada. Os marcadores comuns demonstram que a politica default nao cobre o segredo sintetico nem email, e que rationale, sources e owner ficam fora do scan. A adulteracao descrita acima demonstra uma segunda fronteira sem scan no `apply`. A classificacao de privacidade permanece `falha`.

Nao apareceram arquivos de sessao, caches ou logs adicionais gerenciados pelo candidato nesses fixtures. O tool home continha apenas binario, toolchain, modulos e caches de build Go, todos inventariados separadamente em `final-files.txt`.

### Git, branch e PR

Foram exercitadas tres configuracoes com um arquivo alheio previamente staged e outro modificado sem staging:

| Configuracao | Resultado observado |
|---|---|
| default | HEAD e indice nao foram tocados; pagina e `index.md` ficaram unstaged; os dois arquivos alheios mantiveram seus estados |
| `auto_stage_changes=true` | somente pagina e `index.md` foram adicionados pelo candidato; o staged anterior foi preservado e o unstaged continuou fora do indice |
| `auto_commit=true` | o commit incluiu pagina, `index.md` e tambem o arquivo alheio que ja estava staged; o arquivo unstaged permaneceu fora |

Cada apply levou entre 0,02 s e 0,05 s. `git add` e path-scoped, mas `git commit` consome todo o indice, apesar de a mensagem listar somente os dois caminhos do candidato. Nao ha comandos nativos de branch, push ou PR; simular esses recursos com `git`/`gh` externo nao classificaria agent-memory e foi registrado como `nao suportado`.

Os tres applies Git usaram, respectivamente, os roots `/workspace/cases/git-default`, `/workspace/cases/git-stage` e `/workspace/cases/git-commit`, com os IDs `20260905T024251-refresh-module-stage5-git-default`, `20260905T024251-refresh-module-stage5-git-stage` e `20260905T024252-refresh-module-stage5-git-commit`.

### Testes executados

Todos os testes rodaram offline com os modulos ja provisionados:

```bash
go test ./internal/cli -count=1 -run \
  '^(TestRunPropose_FlagRecordDecisionStages|TestRunPropose_ApplyForcesStagedToLand|TestReviewDiff_ShowsStagedChange|TestRunApply_HappyPath|TestRunReject_RemovesDir|TestRunRebase_SoftDriftNoForceRejected)$'
go test ./internal/memory -count=1 -run \
  '^(TestProposeUpdate_RejectsSecretDetected|TestProposeUpdate_RejectsPII_SSN|TestProposeUpdate_LogsNeverLeakSecretBytes|TestApplyStaged_HappyPath|TestApplyStaged_DriftDetected|TestRebaseStaged_SoftDriftRequiresForce|TestRebaseStaged_SoftDriftForceRebases|TestRebaseStaged_SectionDisappearedIsHardBlock|TestRebaseStaged_NewBaseWithSecretIsRejected|TestMaybeAutoStage_StagesAndCommits|TestMaybeAutoStage_AutoCommitOffStagesButNoCommit|TestApplyStaged_AutoStagesOnApply)$'
go test ./internal/git -count=1 -run \
  '^(TestAddPaths_StagesProvidedFiles|TestCommit_CreatesCommit|TestCommit_NothingStagedIsNoop)$'
go test ./... -count=1
AGENT_MEMORY_BIN=/home/phase-1/bin/agent-memory \
  go test -tags=e2e ./internal/e2e -count=1 -run '^TestRelease01_EndToEnd$' -v
```

Os tres filtros literais completos estao em `harness/agent-memory-stage5-tests.sh`; os nomes e resultados tambem estao preservados em `tests/`.

| Suite | Resultado | Duracao |
|---|---|---:|
| 6 testes focados de CLI | passa | 20,40 s |
| 12 testes focados de memory | passa | 1,09 s |
| 3 testes focados de Git | passa | 0,28 s |
| `go test ./... -count=1` | 20 pacotes passam; 2 sem testes | 8,47 s |
| E2E `TestRelease01_EndToEnd` | 14 subtestes passam | 0,75 s |

A suite e ampla e reproduzivel, mas nao detecta os quatro comportamentos criticos observados no harness: adulteracao do staging, perda de drift fora da secao, marcadores em metadados e commit de staged alheio. Por isso `Testabilidade` avanca de `pendente` para `parcial`, nao para `passa`.

## Etapa 6: integracao com Claude Code

A evidencia foi preservada em `.artifacts/phase-1/stage6-agent-memory`, conforme o [manifesto de evidencias](../evidence-manifest.md). O run usou a revisao fixa limpa, Claude Code 2.1.261, um `HOME` descartavel e nenhum token ou credencial. O fluxo inicial foi compilado offline com Go 1.25.0; a verificacao final recompilou com o Go 1.25.12 exigido pelo protocolo, repetiu a recuperacao MCP e executou o E2E oficial. `agent-memory init` criou o store em um repositorio descartavel e `agent-memory install claude` escreveu somente `.claude/skills/agent-memory/SKILL.md` e `.mcp.json`. O baseline foi commitado antes do fluxo. A passagem usou `env -i`, mas nao Bubblewrap nem duracao por comando, e portanto nao sustenta `passa` sob o protocolo.

Os comandos principais, executados de `/tmp/stage6-agent-memory-run` com o ambiente vazio descrito acima, foram:

```bash
agent-memory init --root /tmp/stage6-agent-memory-run/project --name stage6-agent-memory
agent-memory install claude --root /tmp/stage6-agent-memory-run/project --json
claude -p --session-id 55555555-5555-4555-8555-555555555555 \
  --no-session-persistence --setting-sources project --strict-mcp-config \
  --mcp-config /tmp/stage6-agent-memory-run/project/.mcp.json \
  --output-format stream-json --verbose \
  "Call memory.fetch_context, then report its project name."
python3 evidence/mcp-cycle.py bin/agent-memory project
agent-memory review --root project "$STAGING_ID" --show --diff --json
agent-memory apply --root project "$STAGING_ID" --json
python3 evidence/mcp-retrieve.py bin/agent-memory-go1.25.12 project
GOPROXY=off GOTOOLCHAIN=go1.25.12 AGENT_MEMORY_BIN=bin/agent-memory-go1.25.12 \
  go test -tags=e2e ./internal/e2e -count=1 -run '^TestRelease01_EndToEnd$' -v
```

| Passagem | Codigo | Duracao | Evidencia |
|---|---:|---:|---|
| reinstalacao idempotente | 0 | 0,00 s | `reinstall.json`, `reinstall.time` |
| Claude Code com MCP disponivel | falha de autenticacao esperada | 0,058 s | `claude-session.stream.jsonl` |
| recuperacao em MCP novo, Go 1.25.12 | 0 | 0,05 s | `mcp-retrieve-go1.25.12.json` |
| Claude Code sem binario MCP | falha de autenticacao esperada | 0,055 s | `claude-mcp-unavailable.stream.jsonl` |
| E2E oficial, 14 subtestes | 0 | 0,343 s | stdout preservado na execucao |

Antes do ciclo, o projeto continha os nove arquivos Markdown/scaffold da memoria, a skill, `.mcp.json` e nenhum SQLite. Ao final, `decisions.md` e `index.md` estavam modificados e haviam sido criados `meta/index.sqlite` e `meta/lock.info`; `staging/` estava vazio depois do aceite. stdout e stderr dos dois launches Claude foram preservados separadamente. O fluxo original de proposta/review/apply nao teve cronometro por comando, outra razao para manter `parcial`.

Uma sessao real `claude -p`, com configuracao MCP explicita e `--no-session-persistence`, listou a skill `agent-memory`, os tres tools `memory.fetch_context`, `memory.propose_update` e `memory.status`, e o servidor como `connected`. `apiKeySource=none` fez a chamada do modelo terminar com `authentication_failed`; portanto o run nao atribui ao produto uma chamada que o modelo nao executou.

O mesmo servidor stdio carregado pelo Claude Code foi exercitado por um cliente MCP deterministico. Um primeiro processo chamou `memory.fetch_context` sem query e recebeu 719 caracteres de `conventions.md` e `index.md`. Em seguida, `memory.propose_update` recebeu uma decisao com o marcador `AGENT_MEMORY_STAGE6_OMEGA` e retornou `status=staged`, `human_approval_required=true` e um `staging_id`. A pagina canonica permaneceu intacta ate `review --show --diff` e `apply` serem executados pela CLI; o aceite retornou `status=applied`.

Um segundo processo MCP, sem contexto do primeiro, chamou `memory.fetch_context` com o marcador e recuperou `decisions.md` e o conteudo integral aplicado. O E2E oficial `TestRelease01_EndToEnd` tambem passou em 0,34 s e cobriu instalacao do adaptador, transporte stdio, proposta, revisao, aceite, consulta, rejeicao de segredo e archive. Nao houve arquivo de sessao criado nesse fluxo; ficaram o Markdown canonico, o indice derivado `meta/index.sqlite` e `meta/lock.info`.

Nao existe processo auxiliar permanente a derrubar: cada conexao inicia `agent-memory mcp`. Para observar a falha, uma segunda sessao Claude Code foi iniciada sem o binario no `PATH`. A skill continuou listada, mas o servidor apareceu como `failed`, os tres tools desapareceram e Claude Code continuou ate a falha de autenticacao do modelo. Nao ha spool, retry ou recuperacao automatica. A integracao e funcional e local, mas captura e recuperacao dependem de o modelo obedecer a skill; sem hooks, nao participam garantidamente de todo ciclo de vida.

## Etapa 7: roots separados

O [spike de roots](../spikes/agent-memory-split-roots.md) **passou** com um prototipo descartavel na revisao fixa. A evidencia foi preservada em `.artifacts/phase-1/20260905T170000Z-agent-memory-stage7-evidence`, conforme o [manifesto](../evidence-manifest.md). Uma interface de composicao separou code root, memory repository root e consumer ID; os tres arquivos novos omitidos pelo patch Git original tambem foram preservados no artefato.

Dois repositorios de codigo somente leitura consumiram a mesma memoria sem cross-talk. O trace representativo mostrou Git apenas no repositorio de memoria, e commits, objetos e status dos code roots nao mudaram. Cold start concorrente passou 20 vezes, applies independentes foram preservados, snapshot obsoleto do mesmo arquivo foi rejeitado por drift, o rebuild recuperou tres segundos e `team-b`, e `PRAGMA integrity_check` retornou `ok`. As suites final, race e E2E passaram.

O prototipo precisou corrigir tres defeitos delimitados: `SQLITE_BUSY` no cold start, colisao de staging IDs no mesmo segundo e perda silenciosa entre snapshots do mesmo arquivo. Isso demonstra que a separacao e adaptavel sem redesenhar Markdown ou FTS5, mas a revisao original ainda nao fornece o comportamento e sua orquestracao nao pode ser reutilizada sem mudancas. Privacidade e sessoes nao foram reavaliadas.

## Criterios

| Criterio | Classificacao | Evidencia | Justificativa |
|---|---|---|---|
| Markdown e Git canonicos | passa | [Armazenamento](#armazenamento-e-roots) | Markdown e duravel; SQLite/cache sao derivados. |
| Operacao local | passa | [Etapa 5](#ambiente-e-reproducao) | O fluxo principal de governanca concluiu offline; os demais criterios seguem independentes. |
| Ausencia de backend obrigatorio | passa | [Armazenamento](#armazenamento-e-roots) | MCP stdio e transitorio; nao ha daemon. |
| Indice reconstruivel | parcial | [Etapa 7](#etapa-7-roots-separados) | Rebuild preservou a resposta, mas foi executado no prototipo modificado, nao na revisao fixa intacta. |
| Privacidade antes da persistencia | falha | [Etapa 5](#privacidade) | Marcadores persistiram no staging/lock; apply aceitou segredo adulterado. |
| Sem persistencia de sessoes | falha | [Sessoes](#privacidade-sessoes-e-drift) | Sessions, IDs e metadados sao persistidos localmente. |
| Estado atual | passa | [Etapa 5](#proposta-revisao-e-aplicacao) | Aplicacao explicita deixou somente tres segundos e `team-b`. |
| Consolidacao antes da publicacao | parcial | [Etapa 5](#integridade-e-drift) | Ha staging observavel, mas mutavel, sem revalidacao e contornavel por `--apply`. |
| Revisao explicita | parcial | [Etapa 5](#proposta-revisao-e-aplicacao) | Diff/reject/apply funcionam; nao ha ajuste seguro da mesma proposta. |
| Repositorio de memoria separado | parcial | [Etapa 7](#etapa-7-roots-separados) | A revisao fixa une roots; prototipo delimitado separou memoria e dois consumidores. |
| Integracao com Claude Code | parcial | [Etapa 6](#etapa-6-integracao-com-claude-code) | Skill e MCP carregaram e o ciclo funcional passou, mas nao ha hooks e a ativacao depende do modelo seguir a skill. |
| Testabilidade | parcial | [Etapa 5](#testes-executados) | Suite passa, mas omite quatro falhas criticas demonstradas pelo harness. |
| Licenca compativel | parcial | [Licencas](../licenses.md#agent-memory) | Apache-2.0; transitivas/notices e npm requerem auditoria. |
| Custo de adaptacao | passa | [Etapa 7](#etapa-7-roots-separados) | Seam de roots e tres correcoes concorrentes foram demonstradas e delimitadas. |

## Riscos, custo e duvidas

O spike demonstrou a separacao na composicao de CLI/MCP, o namespacing minimo e as mutacoes Git confinadas a memoria, mas uma interface de producao ainda deve cobrir doctor, instalacao e configuracao persistida. O hotfix 0.5.1 corrige roteamento cruzado anterior (`CHANGELOG.md:8-23`), e a Etapa 7 encontrou riscos adicionais em cold start e staging concorrente. A Etapa 5 mantem a necessidade de tornar staging imutavel ou verificavel, revalidar scan/schema no apply, proteger o arquivo integral contra drift e isolar commits do indice preexistente.

Fluxos restantes limitados:

1. Endurecer staging, revalidacao de scan/schema, drift e isolamento de commit antes de reutilizacao.
2. Delimitar configuracao publica, doctor, instalacao e colisao de consumer IDs.
3. Resposta autenticada do modelo, apenas se a obediencia espontanea a skill puder alterar a decisao.

## Recomendacao preliminar

**Adaptacao seletiva condicionada ao ADR.** Se Go for escolhido, os cinco arquivos do pacote Markdown byte-preserving formam a principal shortlist de fonte. Operacoes por secao, FTS5, staging/diff, lock, merge driver e proveniencia permanecem referencias, mas apply, privacidade, sessoes, drift e commit devem ser substituidos ou endurecidos sob interfaces do produto. O spike de roots passou; a revisao original nao sustenta fork ou base end-to-end. Veja o [mapa de capacidades](../capability-map.md#agent-memory-versus-palinode).
