# T9: E2E sintetico

## Fixed point e seams

O fixed point de T9 e `c42abab89ac5c921f235cb45a5029454c310c7ce`. O harness fica em `tests/e2e`, atravessa somente o binario, MCP stdio, CLI e Git reais, e usa o corpus derivado sob `testdata/phase-2`. Nenhuma interface publica do produto e ampliada para o teste.

O teste focado e `GPM_E2E_EVIDENCE_ROOT=<path> GOPROXY=off go test ./tests/e2e -run TestSyntheticWorkflow -count=1`. A execucao classificatoria e `tests/e2e/run-phase-2.sh`, que cria `.artifacts/phase-2/<run-id>`, executa o fluxo focado offline e depois roda `make verify` e `make verify-offline`. Sem `GPM_E2E_EVIDENCE_ROOT`, o E2E classificatorio e ignorado pelas suites comuns.

## Contrato da evidencia

- O run comportamental usa memory root, code root, HOME, config e state root descartaveis e separados.
- A proposta entra por `memory_propose_replace`; show, apply e publish entram por novos processos CLI.
- A segunda recuperacao usa novo processo MCP, novo cliente, novo HOME, novo code root e novo state root, preservando somente o memory root publicado.
- O run de privacidade e separado, cobre rejeicoes de schema, limits e privacy em corpo e metadados, entrega os marcadores somente em memoria ao Intake Guard e executa o servidor sob trace de operacoes de arquivo.
- A evidencia persistida contem somente configuracao normalizada, versoes, comandos sem payload, codigos, duracoes, hashes, diffs do fixture, inventarios, bytes de contexto, trees Git e resumo sanitizado do trace.
- Os valores literais dos marcadores, payloads MCP e estado temporario de proposta nao entram em `.artifacts`.

## Reproducao

Pre-requisitos: Linux, `strace`, Git e Go 1.25.12 ja provisionado. O runner exige worktree limpo, fixa `GOPROXY=off` no fluxo classificatorio e aceita um run ID opcional:

```bash
tests/e2e/run-phase-2.sh [run-id]
```

O path final da evidencia e impresso em stdout. O diretorio contem:

- `manifest.json`: revisoes, versoes, configuracao normalizada, comandos, codigos, duracoes, hashes, budgets e criterios finais;
- `context-initial.json` e `context-final.json`: `ContextBundle` canonicos e sanitizados;
- `proposal.diff` e `markdown.diff`: diff proposto e diff efetivamente commitado;
- `index-before.txt` e `index-after.txt`: estado completo do indice real;
- `commit-tree.txt`: paths do commit publicado;
- `inventory-before.txt`, `inventory-terminal.txt` e `inventory-after.txt`: paths, modos, tamanhos e hashes, sem copiar objetos Git;
- `privacy-summary.txt`: contagens sanitizadas dos casos rejeitados, do scan de marcadores e de todos os destinos de escrita observados pelo trace;
- `focused-e2e.log`, `fault-injection.log`, `verify.log`, `verify-offline.log` e `verification.tsv`: resultados e duracoes das verificacoes;
- `checksums.sha256`: checksum de cada evidencia preservada.

## Checksums do corpus

```text
0722bf08df9ab6e825bea2c93db875b8159b2fd74cc92c02b84a64dad524043f  testdata/phase-2/memory-initial/services/checkout-api.md
fc1eeb4c4ae74bff0ac295089192508f93f6025db8ac71cd67939b77df57ff05  testdata/phase-2/memory-expected/services/checkout-api.md
b67ab52096dfe95e3afa76965f552b0a55557505e5343f3a5dc5db119413b5ed  testdata/phase-2/code-repo/services/checkout-api/service.yaml
a01d1163d7035b1fdc2784811cf4a589c395b8df710729cb5c36fdfc8582a4b6  docs/phase-1/corpus/code-repo/private-markers.txt
```

## Criterios automatizados

O harness falha se o estado inicial nao retornar cinco segundos e `team-a`, ou se o processo novo nao retornar `services/checkout-api.md`, `checkout-api-operation`, tres segundos e `team-b`. O budget medido e o `json.Marshal(ContextBundle)`, antes do envelope MCP.

Tambem falham: mutacao de Markdown, HEAD ou indice pela proposta; alteracao de anchors; apply/publish nao idempotente; commit com outro path; perda de staged ou unstaged alheio; payload terminal remanescente; schema inesperado ou corpo de sessao em receipts; state root criado pela segunda leitura; marcador no run, trace ou evidencia fora do fixture permitido; ou qualquer destino de escrita depois que os candidatos rejeitados chegam ao servidor traced.

Um run focado de desenvolvimento confirmou 329 bytes na busca e 323 no expand inicial, 328 bytes na busca e 322 no expand final, hash final igual ao fixture esperado, commit contendo somente `services/checkout-api.md`, payload removido, receipts com schema fechado e sem corpo, quatro casos de privacidade rejeitados, zero hits de marcadores fora da fonte e zero destinos de escrita no trace de privacidade. O run classificatorio em revisao limpa e registrado abaixo depois do commit avaliado.

## Run classificatorio

Pendente da revisao limpa deste incremento.
