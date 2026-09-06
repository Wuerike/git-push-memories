# T10: resultado do experimento controlado de valor

## Decisao

**Resultado pre-registrado: `revisar o fluxo`.**

O tratamento preservou correcao, API e escopo em todos os tres pares, mas atingiu a reducao minima de 25% em chamadas de exploracao em somente um par. O protocolo exigia pelo menos dois. Portanto, o experimento nao autoriza progressao para a Fase 3, mesmo que a soma agregada tenha caido de 16 para 12 chamadas, exatamente 25%.

Essa leitura nao foi alterada depois dos resultados. Um agregado favoravel nao substitui a regra por pares congelada em `experiment.json`.

## Identidade do run

| Campo | Valor |
|---|---|
| experiment ID | `parcel-dispatch-value-v3` |
| product revision | `ca166d18dd47a81e46a5b22671763545280a7a34` |
| protocol hash | `242409c12a93d681fc9bde8c6f8511ba98ccf5b9c6d5fbf189a532b1a0b85e18` |
| evidence root local | `.artifacts/phase-2/20260906T222652Z-value-experiment` |
| report checksum | `e9b1af70e6415337794d9c160489b68d78aee03bafd5920cf21736732cc20445` |
| checksum inventory checksum | `1e0f32e6095de0743723f94261d9aa3d3511665c3e9883afa6a048c0c286b187` |
| model alias | `sonnet` |
| modelo observado | `claude-sonnet-5` em sete sessoes |
| Claude Code | 2.1.263 |
| Go | 1.25.12 |
| Git | 2.54.0 |
| MCP SDK | 1.7.0 |

O worktree do produto estava limpo. Todas as sessoes retornaram uma resposta autenticada, usaram model ID identico e ficaram abaixo do limite individual de USD 1.

## Sessao A

O runner aceitou a sessao A somente depois de observar uma resposta autenticada, exatamente uma chamada a `memory_propose_replace`, code root limpo e uma proposta cujo replacement e source paths correspondiam ao post-state pre-registrado. Essa validacao ocorreu antes de apply e publish conforme o runner congelado em `ca166d1`; o texto da resposta e o resultado bruto da tool foram descartados.

Resultados:

- seis chamadas de exploracao: cinco `Read` e um `Grep`;
- uma chamada a `memory_propose_replace`;
- oito turnos;
- 23.979 ms;
- custo reportado de USD 0,0608366;
- commit de memoria restrito a `domains/parcel-dispatch.md`;
- contexto publicado com 387 bytes, abaixo do budget de 2.048 bytes;
- marker scan com zero hits.

O memory tree mudou de `7aa75dba88527172d30d49832d70aaa296e8d32a` para `17fe345f1a289caf04079f6aa034eee0511f2322`. O diff publicado teve hash `b44dbf48fb2f616ee6439783f969b88ebbfb48dfc02ff91152e3384eee0c4960`.

## Rubrica cega

O runner avaliou os seis clones como `run-01` a `run-06` sem passar o arm para a funcao de avaliacao e somente depois juntou o mapa treatment/control. A evidencia sanitizada preserva resultados opacos seguidos do mapa. Todos obtiveram 10/10:

- hidden tests passaram;
- public tests passaram;
- API exportada permaneceu identica;
- mudancas ficaram limitadas a `dispatch/policy.go` e `dispatch/policy_test.go`;
- correctness gate passou verdadeiro.

Tres hashes de diff distintos apareceram entre os seis resultados, mas todos satisfizeram a mesma rubrica. O isolamento entre runs e uma propriedade do runner versionado: cada processo recebeu somente seu clone em Bubblewrap; respostas e workspaces anteriores nao foram montados no sandbox seguinte. O report nao preserva streams para uma verificacao independente do conteudo descartado.

## Comparacao por par

| Par | Treatment | Control | Exploracao T | Exploracao C | Melhora | Limiar | Scores |
|---:|---|---|---:|---:|---:|---|---|
| 1 | `run-02` | `run-01` | 5 | 6 | 16,7% | nao | 10 / 10 |
| 2 | `run-03` | `run-04` | 2 | 5 | 60,0% | sim | 10 / 10 |
| 3 | `run-06` | `run-05` | 5 | 5 | 0,0% | nao | 10 / 10 |

Todos os tres treatments registraram uma chamada a `memory_search` e `memory_context_relevant=true`; esse booleano e calculado pelo parser somente quando o `tool_result` ligado ao mesmo `tool_use_id` contem timeout, queue e owner esperados. Nenhum control registrou chamada a uma tool de memoria. Os resultados brutos foram deliberadamente descartados, portanto a evidencia preservada sustenta essa verificacao executada pelo runner, nao uma reinspecao independente do payload.

## Sinais secundarios

| Medida | Treatment | Control |
|---|---:|---:|
| chamadas de exploracao, soma | 12 | 16 |
| duracao, soma | 501.766 ms | 153.266 ms |
| custo reportado, soma | USD 0,2335408 | USD 0,2160580 |

O custo total das sete sessoes foi USD 0,5104354. Duracao e custo nao eram a metrica primaria e nao alteram a decisao, mas o tratamento foi substancialmente mais lento no agregado. Os runs treatment 1 e 2 demoraram 232.233 ms e 214.943 ms, enquanto os demais ficaram entre 36.963 ms e 66.848 ms. Sem respostas ou traces brutos, nao ha evidencia suficiente para atribuir causalmente essa latencia a uma etapa especifica.

## Isolamento e privacidade

Os itens abaixo descrevem controles do runner versionado e executado em `ca166d1`. O report preserva configuracao, hashes e booleanos finais, mas nao preserva credenciais, mounts brutos ou streams que permitiriam repetir uma auditoria forense do processo.

- Cada sessao usou processo, clone, HOME, config, state e `/tmp` novos.
- Bubblewrap montou somente o workspace corrente; memoria apareceu apenas no treatment e ficou read-only nos runs da tarefa.
- O evaluator oculto e os worktrees anteriores nao foram montados nas sessoes.
- A credencial OAuth foi copiada com modo `0600` apenas para tmpfs e removida depois de cada processo.
- Os diretorios temporarios de auth e experimento nao permaneceram apos o runner.
- `--no-session-persistence`, MCP estrito e allow/deny lists foram aplicados.
- Nenhum prompt, resposta, transcript, stream Claude, stderr, diff bruto ou credencial foi preservado.
- A evidencia local contem somente `report.json` e `checksums.sha256`.

## Verificacao tecnica

O shell registrado executou os cinco checks antes de criar o ledger que autoriza a inferencia. O report incorporou o TSV resultante, com todos os checks em exit 0:

| Check | Duracao |
|---|---:|
| `go test ./...` | 459 ms |
| `go test -race ./...` | 478 ms |
| `go vet ./...` | 198 ms |
| `go build ./...` | 405 ms |
| fault injection/recovery | 1.062 ms |

## Tentativas anteriores

V1 e v2 abortaram durante preflight, antes de qualquer processo Claude ou outcome. V1 deixou evidence root vazio. V2 registrou somente a falha de `go test ./...` quando chamado recursivamente de dentro do runner; o mesmo comando passou diretamente no shell. Seus ledgers `started` foram preservados e nenhum protocolo foi repetido.

V3 moveu o preflight para o shell, alterou experiment ID, seed e ordem, e manteve hipotese, tarefa, rubrica e limiar. Essa mudanca foi commitada antes da execucao.

## Consequencia

O gate tecnico da Fase 2 foi demonstrado por T9. Em T10, os treatments observaram menor exploracao no agregado e correcao equivalente, mas somente um par atingiu o limiar; isso nao demonstra um efeito causal consistente sob a regra pre-registrada. Conforme o [gate da Fase 2](plan.md#gate-de-conclusao), a decisao e **nao iniciar a Fase 3 ainda**.

Um proximo experimento deve usar novo protocolo e investigar a friccao do tratamento sem reutilizar seletivamente estes runs. Hipoteses apropriadas para pre-registro futuro incluem reduzir cold start do MCP, medir separadamente latencia de tool e inferencia, e testar uma tarefa em que a memoria substitua mais de uma leitura de evidencia dispersa. Esses pontos sao propostas futuras, nao reinterpretacoes do resultado atual.
