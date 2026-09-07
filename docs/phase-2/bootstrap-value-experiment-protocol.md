# T19: experimento autenticado apos bootstrap explicito

## Hipotese e fixed point

T19 repete a hipotese, tarefa posterior, rubrica cega e limiar de T10. A unica mudanca causal intencional e a sessao A: em vez de substituir uma secao preexistente, ela inicia em um repositorio Git de memoria sem Markdown, executa `/remember`, recebe a escolha humana explicita `create a root page`, recebe uma confirmacao humana separada e cria uma unica proposta `create_page`. Apply e publish continuam sendo comandos humanos executados pelo harness somente depois da validacao exata da proposta.

Como T19 deve produzir exatamente um commit, protocolo, runner e resultado serao commitados juntos. Antes de qualquer inferencia, o shell grava um receipt append-only local com revision base, hash do protocolo, hash das fontes executadas, aprovacao e evidence root; o runner recusa divergencia desses hashes. Essa evidencia demonstra fixacao anterior ao outcome, mas nao equivale a um timestamp externo imutavel.

## Cenario congelado

- Code seed, task, task system prompt, rubric e evaluator sao os mesmos fixtures sinteticos de T10.
- A entrada `/remember` fornece um unico fato: priority parcels escalam apos 90 segundos por `route-amber`, cuja fila ativa pertence a `routing-ops`; os source paths sao `deploy/production-alerts.yaml` e `ops/queues.yaml`.
- A busca inicial deve retornar zero handles porque o memory root nao contem Markdown.
- A primeira resposta humana e exatamente `create a root page`.
- A segunda resposta humana ocorre somente depois da apresentacao da operacao e e exatamente `confirm`.
- A proposta aceita deve ser `create_page`, path `parcel-dispatch.md`, title `Parcel dispatch`, body com os dois bullets congelados e os dois source paths registrados. Antes de apply, o post-state hash da proposta deve corresponder byte a byte ao fixture canonico `memory-expected/parcel-dispatch.md`, incluindo heading e anchors gerados pelo produto.

## Desenho e decisao

- Claude Code 2.1.263, alias `sonnet`, effort `medium`, autocompact `100k`, no maximo USD 1 por sessao.
- Uma sessao A interativa e exatamente tres pares treatment/control, com processo, clone, HOME, config, state e credencial tmpfs novos por sessao.
- Ordem derivada por SHA-256 de `gpm-bootstrap-value-v1:<pair>`; o primeiro byte impar coloca treatment primeiro.
- Treatment recebe a memoria criada e somente search/expand; control recebe MCP vazio.
- Streams, prompts resolvidos, respostas, credenciais e workspaces descartaveis nao sao persistidos. A evidencia sanitizada preserva somente metricas, hashes, rubrica cega e booleans de conformidade do fluxo `/remember`.
- Metrica primaria: duracao monotonic start-to-exit (`task_duration_ms`) da tarefa posterior. O limiar e reducao de pelo menos 25%; chamadas `Read`, `Grep` e `Glob` permanecem sinal secundario para comparacao descritiva com T10, sem combinar amostras.
- `continuar` exige treatment correto nos tres pares, score nunca inferior ao control e reducao de pelo menos 25% em pelo menos dois pares.
- Correcao preservada com limiar insuficiente ou falha operacional resulta em `revisar o fluxo`; regressao de correcao ou score resulta em `nao demonstrou valor`.
- Somente `continuar` libera a Fase 3.

## Aprovacao e execucao

Antes do run autenticado, uma pessoa deve aprovar explicitamente este protocolo, incluindo o limite total de ate USD 7 e o envio dos fixtures sinteticos ao provedor do modelo. O comando sera:

```bash
tests/experiment/run-phase-2-bootstrap-value.sh --approved
```

Depois do run unico, o report sanitizado sera registrado em `docs/phase-2/bootstrap-value-experiment-report.md` sem reinterpretar a regra congelada.
