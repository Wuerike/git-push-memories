# T10: experimento controlado de valor

## Fixed point e objetivo

O fixed point de T10 e `69a27b3b2bebb75b1ebbec98609ec58ded9144b8`. O experimento usa somente fixtures sinteticos versionados sob `testdata/phase-2/value-experiment`; nenhum repositorio, nome, fato ou tarefa privada participa. "Real" significa que as sete sessoes usam uma resposta autenticada do Claude Code, enquanto cenario, avaliacao e isolamento permanecem controlados e reproduziveis.

A hipotese e que uma memoria curta do estado operacional atual reduz exploracao de codigo sem regredir a correcao de uma tarefa posterior.

## Cenario congelado

O code seed e um modulo Go pequeno de despacho de encomendas. `dispatch.EscalationFor` preserva comportamento para tiers standard e desconhecido, mas a politica priority ainda nao esta implementada. A regra atual esta distribuida entre `deploy/production-alerts.yaml` e `ops/queues.yaml`; `docs/legacy-policy.md` contem uma regra anterior explicitamente aposentada.

A sessao A deve distinguir estado atual de historico e propor a pagina `domains/parcel-dispatch.md`, secao `parcel-dispatch-priority-escalation`. O post-state exato esta congelado em `memory-expected`. A tarefa posterior pede a implementacao da politica atual sem fornecer timeout, queue ou owner.

## Desenho experimental

- Modelo pre-registrado: alias `sonnet`, effort `medium`, budget maximo de USD 1 por sessao.
- Exatamente tres pares, seis clones locais sem hardlinks no mesmo code commit e uma sessao A separada; cada clone fornece um worktree autocontido ao sandbox.
- Cada run recebe HOME, XDG config, Claude config, state root e processo novos.
- A credencial OAuth existente e copiada com modo `0600` somente para um diretorio `tmpfs` por run e removida assim que o processo termina. Credencial, config e estado de sessao nunca entram na evidencia.
- Claude Code usa `--no-session-persistence`, MCP estrito, permission prompts desabilitados e allowlist de tools. Nao sao carregados hooks, skills ou settings do usuario/projeto.
- Tratamento recebe `memory_search`/`memory_expand` sobre a memoria publicada e nega `memory_propose_replace`. Controle recebe um MCP config vazio. Prompt, system prompt, modelo, tools de codigo e budgets sao identicos.
- A ordem v3 vem de SHA-256 de `gpm-value-v3b:<pair>`: primeiro byte impar coloca tratamento primeiro; primeiro byte par coloca controle primeiro. Os tres resultados congelados sao controle-tratamento, tratamento-controle e controle-tratamento.
- Respostas e eventos Claude sao consumidos em memoria. O parser retira somente modelo, duracao, custo, turnos e contagens de tools; texto, inputs e resultados nao sao gravados.

## Avaliacao cega

Os worktrees usam IDs opacos `run-01` a `run-06`. Primeiro o evaluator injeta testes externos, executa a rubrica e registra resultados sem receber o arm. Somente depois o runner junta o mapa pre-registrado de assignments e calcula os pares.

A rubrica vale dez pontos:

| Check | Pontos |
|---|---:|
| testes ocultos da politica atual | 4 |
| testes publicos | 2 |
| API exportada compila sem alteracao | 2 |
| diff limitado a Go sob `dispatch/` | 2 |

O correctness gate exige testes ocultos, testes publicos e API preservada. A metrica primaria e o numero de chamadas `Read`, `Grep` e `Glob`; busca de memoria, edicao e execucao de testes nao contam como exploracao de codigo.

O limiar e reducao de pelo menos 25% em dois dos tres pares. `continuar` exige treatment correto em todos os pares, score nunca inferior ao control e limiar atingido. Falha operacional ou limiar insuficiente com correcao preservada resulta em `revisar o fluxo`. Regressao de correcao ou score resulta em `nao demonstrou valor`.

## Integridade e privacidade

`experiment.json` registra seed, ordens, configuracao, regra de decisao e hashes de task, system prompts, rubrica, memoria, policy inicial e evaluator. O teste `TestValueExperimentProtocolIsFrozenAndSelfConsistent` recalcula esses hashes e prova que o fixture comeca com testes publicos verdes, evaluator vermelho e anchors estaveis.

O runner recusa worktree sujo e evidence root existente. Um ledger local impede repetir seletivamente o mesmo protocolo depois de observar resultados. Runs extras exigem novo experiment ID, seed, protocolo e commit.

A evidencia preserva `report.json` e `checksums.sha256`. Nao preserva prompt, resposta, transcript, stream Claude, stderr, credencial, config temporaria, state root, diff bruto ou conteudo de arquivos modificados. Os cinco checks rodam diretamente no shell antes do ledger; seu TSV sanitizado e entregue ao runner e incorporado ao report. O report contem configuracao normalizada, comandos e duracoes separados para testes, race, vet, build e fault injection, versoes, criterio de ranking, hashes dos fixtures, inventario inicial de paths, trees/index/diff da memoria, paths e hashes dos resultados e a rubrica cega.

## Tentativas operacionais anteriores

O protocolo `parcel-dispatch-value-v1`, congelado em `c56dc3d3bd9f6e7d81079da4f6667081ccfa8792`, abortou no preflight antes de iniciar qualquer processo Claude. Seu evidence root permaneceu vazio. V2, congelado em `8c005e9735d6577ed56f55e0ad969631017b1785`, tambem abortou antes de inferencia e registrou que `go test ./...` falhou somente quando lancado recursivamente pelo proprio runner; o mesmo comando passou diretamente no shell. Nenhum outcome foi observado e ambos os ledgers permanecem `started`.

V3 move os cinco checks para o shell anterior ao ledger e fornece ao runner somente um TSV sanitizado. Ela altera experiment ID, seed e ordem, sem mudar hipotese, tarefa, rubrica ou limiar.

## Execucao

Depois que este protocolo estiver revisado e commitado em worktree limpo:

```bash
tests/experiment/run-phase-2-value.sh
```

O runner pode consumir ate USD 7 pelo limite pre-registrado, embora ambientes por assinatura possam nao reportar custo marginal. A execucao termina sem publicar nada fora dos repositorios Git descartaveis locais.

O hash atual de `experiment.json` e `242409c12a93d681fc9bde8c6f8511ba98ccf5b9c6d5fbf189a532b1a0b85e18`. Resultados serao registrados em `docs/phase-2/value-experiment-report.md` somente depois de concluida a avaliacao pre-registrada.
