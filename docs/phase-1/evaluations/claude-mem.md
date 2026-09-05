# Avaliacao de claude-mem

## Identificacao

| Campo | Valor |
|---|---|
| Projeto | claude-mem |
| Repositorio | `https://github.com/thedotmack/claude-mem.git` |
| Clone local | `~/workspace/claude-mem` |
| Revisao | `be44b6c8e238a7e2bc5b3403c05afac071a59ead` |
| Branch | `main`, alinhada a `origin/main` |
| Estado inicial | arvore limpa |
| Data | 2026-09-04 (triagem estatica); 2026-09-05 (Etapa 6) |
| Avaliacao | triagem estatica e integracao comportamental com Claude Code |
| Avaliador | OpenCode |
| Run ID | `stage6-claude-mem-run` |
| Commit inicial da memoria | nao se aplica; o canonico do candidato e SQLite |

## Hipotese

claude-mem e uma referencia forte para hooks e recuperacao progressiva, mas SQLite/sessoes canonicos, daemon e publicacao automatica o eliminam como base da arquitetura Markdown/Git.

## Escopo e metodo

Foram inspecionados manifestos, hooks, adaptadores, worker, stores SQLite/Postgres, sync, MCP, telemetria e testes. Na Etapa 6, os artefatos empacotados foram executados com Bun, SQLite e provider deterministico em loopback; Chroma, cloud sync e telemetria permaneceram desabilitados. A suite fonte nao foi executada.

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

Os comandos retornaram `0`; a duracao nao foi medida. A stack e TypeScript/ESM, Node 20.12+, Bun, Express, React, MCP, SQLite, Chroma opcional e, no runtime server, Postgres/BullMQ/Redis (`package.json:28-55,121-183`). O historico local tem 2.525 commits, 147 desde 2026-08-05 e multiplos contribuidores; a revisao e de 2026-09-03, versao 13.24.0.

Ambiente usado na triagem: Linux x86_64 e Git 2.54.0, conforme [`evaluation-protocol.md`](../evaluation-protocol.md#ambiente-de-referencia). Somente Git e leitura estatica de arquivos foram usados.

## Fluxo estatico

| Estagio | Evidencia no clone | Conclusao |
|---|---|---|
| Hooks | `plugin/hooks/hooks.json:4-88`; `src/cli/hook-command.ts:85-100`; `src/cli/adapters/claude-code.ts:8-40` | Captura SessionStart, prompt, tool use, Read e Stop. |
| Filtro | `src/utils/tag-stripping.ts:4-49`; `src/services/worker/http/shared.ts:102-136` | Remove tags `<private>` e conteudo injetado, mas nao faz scan geral de segredo/PII. |
| Persistencia | `src/services/sqlite/SessionStore.ts:925-993,2417-2742` | SQLite guarda sessoes, prompts, observacoes e resumos. |
| Processamento | `src/sdk/prompts.ts:24-189`; `src/sdk/parser.ts:41-186`; `src/services/worker/agents/ResponseProcessor.ts:406-465` | LLM produz XML, parseado e inserido imediatamente. |
| Publicacao | `src/services/worker/agents/ResponseProcessor.ts:598-759` | Depois do insert, sincroniza Chroma/cloud e pode atualizar CLAUDE.md. |
| Recuperacao | `src/cli/handlers/context.ts:28-87,147-153`; `src/servers/mcp-server.ts:438-559` | SessionStart injeta indice; MCP progride por search, timeline e observacoes completas. |

## Armazenamento e dependencias

SQLite e a fonte de observacoes, sessoes, prompts e resumos. FTS e reconstruivel dessas tabelas e Chroma pode fazer backfill (`src/services/sqlite/SessionSearch.ts:41-151`; `src/services/sync/ChromaSync.ts:1070-1121`). O bloco Markdown e opcional, incompleto e desabilitado por padrao (`src/shared/SettingsDefaultsManager.ts:165-182`; `src/utils/claude-md-utils.ts:315-368`); nao reconstrui o store. No runtime server, Postgres e canonico.

O fluxo principal requer worker HTTP Bun e SQLite. A geracao exige Claude/Anthropic, Gemini, OpenRouter ou endpoint OpenAI-compatible; Chroma e desativavel, mas e padrao e inicia `uvx`/Python (`src/services/sync/ChromaMcpManager.ts:187-256,573-580`). MCP tambem depende ou inicia o worker (`src/servers/mcp-server.ts:408-435`).

## Privacidade, sessoes e telemetria

`normalizeStoredPromptText` volta ao prompt original quando a remocao de `<private>` deixa texto vazio (`src/services/sqlite/prompt-storage.ts:6-20`). A rota cria a sessao antes de detectar esse caso (`src/services/worker/http/routes/SessionRoutes.ts:596-631`), e ausencia de prompt autoriza eventos posteriores (`src/services/worker/validation/PrivacyCheckValidator.ts:9-49`). Formatadores tambem podem logar comandos/queries antes do worker (`src/utils/logger.ts:156-282`). No runtime server, o evento bruto entra em Postgres antes da filtragem (`src/server/services/IngestEventsService.ts:90-147`; `src/storage/postgres/schema.ts:169-185`).

PostHog e opt-out, com host padrao e ID persistente (`src/services/telemetry/consent.ts:35-63,123-143`; `src/services/telemetry/common.ts:14-29`). `SECURITY.md:148-157` afirma ausencia de telemetria, em conflito com o codigo e `docs/public/telemetry.mdx:1-16`.

## Estado atual, proposta e Claude Code

A deduplicacao usa hash de sessao, titulo e narrativa; nao resolve contradicoes entre sessoes (`src/services/sqlite/observations/store.ts:5-13`; `src/services/sqlite/SessionStore.ts:2662-2737`). O XML estruturado e transitorio, mas e aplicado imediatamente, sem diff, ajuste, rejeicao ou aceite humano. Nao existe publicacao Git.

A integracao de Claude Code e substantiva. SessionStart injeta indice compacto, PreToolUse:Read recupera contexto de arquivo, e MCP permite aprofundamento progressivo. Defaults incluem ate 50 observacoes e dez sessoes (`src/shared/SettingsDefaultsManager.ts:135-169`). A recuperacao por arquivo deduplica por sessao e limita a 15 itens (`src/cli/handlers/file-context.ts:14-86,196-272`). A Etapa 6 confirmou o ciclo de hooks, injecao e aprofundamento progressivo.

## Testes encontrados

Ha testes de tag stripping (`tests/utils/tag-stripping.test.ts:22-104`), persistencia de prompts (`tests/sqlite/session-store-prompts.test.ts:21-55`), contratos dos hooks (`tests/infrastructure/plugin-distribution.test.ts:385-407`), contexto read-only (`tests/context/context-builder-readonly.test.ts:137-197`), parser XML (`tests/sdk/parser.test.ts:36-159`), isolamento do observador (`tests/security/observer-tool-enforcement.test.ts:30-158`) e telemetria (`tests/telemetry/consent.test.ts:26-102`). O E2E de hook simula HTTP, nao uma sessao real (`tests/integration/hook-execution-e2e.test.ts:23-128,210-238`). A suite fonte nao foi executada; a Etapa 6 exercitou os artefatos empacotados.

## Etapa 6: integracao com Claude Code

A evidencia foi preservada em `.artifacts/phase-1/stage6-claude-mem`, conforme o [manifesto de evidencias](../evidence-manifest.md). A copia executavel veio de `git archive` da revisao fixa e usou os artefatos empacotados em `plugin/`, Node 20.20.0, Bun 1.3.11, Claude Code 2.1.261 e `zod` 4.4.3 instalado do cache npm com `--offline`. `HOME`, `CLAUDE_CONFIG_DIR` e `CLAUDE_MEM_DATA_DIR` eram descartaveis. Um provider OpenAI-compatible deterministico ficou somente em loopback; Chroma, cloud sync, transcript watcher, Telegram, folder CLAUDE.md e telemetria foram desabilitados. Nenhuma credencial real foi fornecida. A passagem nao usou Bubblewrap nem registrou duracao por comando; eventos posteriores a falha de autenticacao foram dirigidos pelo harness. Essas limitacoes restringem a classificacao a `parcial`.

Os comandos principais, executados de `/tmp/stage6-claude-mem-run` com o ambiente vazio e as variaveis descritas acima, foram:

```bash
npm install --offline --prefix /tmp/stage6-claude-mem-run/deps \
  --ignore-scripts --no-audit --no-fund
node pkg/plugin/scripts/bun-runner.js pkg/plugin/scripts/worker-service.cjs start
claude -p --session-id 77777777-7777-4777-8777-777777777777 \
  --no-session-persistence --setting-sources user \
  --settings /tmp/stage6-claude-mem-run/home/.claude/settings.json \
  --plugin-dir /tmp/stage6-claude-mem-run/pkg/plugin \
  --output-format stream-json --include-hook-events --verbose \
  "Initialize the synthetic claude-mem integration session."
node pkg/plugin/scripts/bun-runner.js pkg/plugin/scripts/worker-service.cjs \
  hook claude-code observation < evidence/post-tool-use.json
node pkg/plugin/scripts/bun-runner.js pkg/plugin/scripts/worker-service.cjs \
  hook claude-code context < evidence/session-b-context-input.json
NODE=/home/Wuerike.Cavalheiro/.nvm/versions/node/v20.20.0/bin/node
python3 evidence/mcp-client.py "$NODE" pkg/plugin/scripts/mcp-server.cjs \
  evidence CLAUDE_MEM_STAGE6_OMEGA
```

| Passagem | Codigo | Duracao | Evidencia |
|---|---:|---:|---|
| restart do worker | 0 | 1,84 s | `worker-restart.*` |
| Claude Code com plugin | falha de autenticacao esperada | 0,391 s | `claude-session.stream.jsonl` |
| SessionStart repetido | 0 | 0,39 s | `context-repeat.*` |
| `search -> timeline -> get_observations` | 0 | 0,20 s | `mcp-repeat.json`, `mcp-repeat.time` |
| MCP sem worker | resposta `isError=true` | nao registrada | `mcp-unavailable.json` |
| hook sem Bun | 1 | nao registrada | `hook-unavailable.stderr` |

Antes do start, o data dir continha apenas `settings.json`. Ao final havia SQLite com WAL/SHM, backup, log, health, supervisor e marcadores de migracao/clean shutdown; stdout e stderr dos launches e hooks foram preservados separadamente. A ausencia de tempo nas duas passagens de falha reforca a classificacao `parcial`.

Uma sessao `claude -p --plugin-dir ... --no-session-persistence` carregou o plugin real. Os dois comandos de `SessionStart` retornaram zero: o primeiro confirmou o worker pronto e o segundo injetou `No previous sessions found.`. `UserPromptSubmit` tambem retornou zero. Claude Code listou o servidor `plugin:claude-mem:mcp-search` como `connected` e expos os tools de busca, timeline e observacoes, alem das skills do plugin. Como `apiKeySource=none`, a chamada principal terminou com `authentication_failed`; o run nao usa uma resposta do modelo como evidencia.

O payload sintetico de `PostToolUse` foi enviado pelo dispatcher empacotado `bun-runner.js -> worker-service.cjs hook claude-code observation`. O worker chamou o provider local, persistiu uma observacao `discovery` com o marcador `CLAUDE_MEM_STAGE6_OMEGA` e retornou `{}` sem ruído em stderr. Um novo payload de `SessionStart`, com outro ID de sessao e sem o contexto anterior, recebeu um indice de 525 bytes e 67 palavras contendo ID, tipo e titulo da descoberta.

O aprofundamento usou o servidor MCP empacotado em um processo novo. `search` retornou um resultado e o ID `#1`; `timeline` recuperou a janela com o item marcado como anchor; `get_observations([1])` devolveu fatos, narrativa, conceitos, modelo gerador e o `memory_session_id`. Assim, foram necessarias tres chamadas para passar do termo de busca ao conteudo completo, enquanto `SessionStart` forneceu apenas o indice compacto.

O inventario encontrou `claude-mem.db` mais WAL/SHM, backup automatico, log diario, `observer-health.json`, `supervisor.json`, PID e marcadores de migracao. O banco continha uma sessao, um prompt e uma observacao, alem de tabelas FTS, outbox e sync. A observacao manteve um `memory_session_id` derivado do ID Claude, projeto, corpo e metadados do provider. Isso confirma diretamente que prompts, sessoes e observacoes sao canonicos, mesmo quando Claude Code usa `--no-session-persistence`. Nao foram criados diretorio Chroma nem estado de telemetria.

Na degradacao, o worker foi encerrado. O servidor MCP ainda inicializou, mas `search` retornou `isError=true` com `Error calling Worker API: fetch failed`. Sem Bun no `PATH`, o launcher do hook encerrou com codigo 1 e a mensagem `Bun not found`; nao houve contexto ou recuperacao local alternativa. O produto tenta iniciar o worker automaticamente quando Bun esta disponivel, mas worker e runtime continuam dependencias obrigatorias.

## Criterios

| Criterio | Classificacao | Evidencia | Justificativa estatica |
|---|---|---|---|
| Markdown e Git canonicos | falha | [Armazenamento](#armazenamento-e-dependencias) | SQLite/Postgres sao canonicos; Markdown e incompleto e nao ha Git. |
| Operacao local | pendente | [Dependencias](#armazenamento-e-dependencias) | Endpoint LLM local e possivel; falta execucao offline. |
| Ausencia de backend obrigatorio | falha | [Dependencias](#armazenamento-e-dependencias) | Worker HTTP e obrigatorio. |
| Indice reconstruivel | falha | [Armazenamento](#armazenamento-e-dependencias) | Indices voltam do SQLite, nao do Markdown exigido. |
| Privacidade antes da persistencia | falha | [Privacidade](#privacidade-sessoes-e-telemetria) | Prompt/evento bruto e logs podem preceder a filtragem. |
| Sem persistencia de sessoes | falha | [Sessoes](#privacidade-sessoes-e-telemetria) | IDs, prompts, observacoes e resumos sao canonicos. |
| Estado atual | falha | [Estado atual](#estado-atual-proposta-e-claude-code) | Modelo aditivo sem supersessao semantica. |
| Consolidacao antes da publicacao | parcial | [Proposta](#estado-atual-proposta-e-claude-code) | XML existe antes do insert, mas nao e proposta observavel. |
| Revisao explicita | falha | [Proposta](#estado-atual-proposta-e-claude-code) | Nao ha gate humano antes da escrita. |
| Repositorio de memoria separado | parcial | [Armazenamento](#armazenamento-e-dependencias) | Data dir serve varios projetos, mas nao e repositorio Git. |
| Integracao com Claude Code | parcial | [Etapa 6](#etapa-6-integracao-com-claude-code) | Plugin, hooks e MCP carregaram; captura e recuperacao passaram no harness, mas nao entre duas sessoes autenticadas nem no sandbox do protocolo. |
| Testabilidade | pendente | [Testes](#testes-encontrados) | Suite ampla, ainda nao executada. |
| Licenca compativel | parcial | [Licencas](../licenses.md#claude-mem) | Nucleo Apache; Cowork, ativos e transitivas exigem delimitacao. |
| Custo de adaptacao | passa | [Custo](#riscos-custo-e-duvidas) | Mudancas estao delimitadas; custo alto. |

## Riscos, custo e duvidas

Virar base exigiria trocar o canonico por Markdown/Git, retirar sessoes, remover daemon, introduzir proposta/revisao, consolidar estado atual, filtrar antes de logs/DB e desabilitar telemetria/egress por padrao. Isso torna fork antieconomico.

O marketplace inclui `claude-mem-cowork`, cujo manifesto declara PolyForm-Noncommercial (`cowork/.claude-plugin/plugin.json:1-17`); deve ficar fora de qualquer reutilizacao. Logos, fontes e bundles tambem exigem auditoria.

Fluxos restantes limitados:

1. Passagem de privacidade: prompt totalmente privado, PostToolUse e Stop sob trace, com LLM/telemetria sinteticos.
2. Rebuild apenas de Chroma a partir de SQLite, como referencia de degradacao.
3. Spike apenas se houver interesse em extrair adaptador/hook sem worker, sessoes e telemetria.

## Recomendacao preliminar

**Referencia.** Hooks, disciplina de IO, startup/degradacao e recuperacao progressiva devem ser reimplementados conceitualmente; o armazenamento e processamento centrais nao atendem as restricoes.
