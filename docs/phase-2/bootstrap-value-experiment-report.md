# T19: resultado do experimento autenticado apos bootstrap

## Decisao

**Resultado pre-registrado: `revisar o fluxo`.**

O unico run autorizado iniciou, mas a sessao A nao satisfez o gate autenticado de criacao por `/remember`. O runner encerrou antes de apply, publish e dos seis runs treatment/control. Nao existem pares validos, portanto nao ha evidencia de reducao de duracao e a Fase 3 permanece bloqueada.

Essa classificacao aplica sem reinterpretacao a regra congelada: friccao operacional que invalida o experimento antes de um resultado comparavel produz `revisar o fluxo`. O run nao sera repetido sob o mesmo experiment ID.

## Identidade

| Campo | Valor |
|---|---|
| experiment ID | `parcel-dispatch-bootstrap-value-v1` |
| revision base | `1b7590c440486a65612401a8dbcd02740f0a85e9` |
| protocol hash | `3f6875688cca5ca0bcce5e88ee686ab08e5d50c7f88620ca1c71fb6bbcabfc8d` |
| source hash pre-run | `cf6352a2cf8b66dd8fbb2b229386d70e995d2cec05d1bd5266ca990f77e488b6` |
| ledger checksum | `3e1b7edcdf488a2f393780a26110a83c45917467e81843496501c4b865f93d63` |
| evidence root local | `.artifacts/phase-2/20260907T141709Z-bootstrap-value-experiment` |
| Claude Code | `2.1.263` |
| modelo pre-registrado | alias `sonnet`, effort `medium` |
| limite | ate USD 1 por sessao; ate USD 7 no protocolo completo |

O protocolo havia sido aprovado explicitamente antes do comando, e o ledger exclusivo registrou `status=started` antes da tentativa autenticada. Como o run abortou, ele deliberadamente nao foi reescrito como `completed`.

## Resultado observado

O shell alcancou a criacao do ledger, o que implica que os cinco preflights anteriores retornaram exit code zero: tests, race, vet, build e fault injection. O runner entrou na etapa da sessao A e retornou somente:

```text
authenticated discovery session failed
```

Esse erro e intencionalmente sanitizado. Ele prova que a etapa nao passou, mas nao distingue falha ao iniciar ou conduzir o processo de uma rejeicao dos gates do fluxo. As possibilidades incluem:

- inicio, streaming ou encerramento normal do processo Claude isolado;
- conclusao da sessao Claude autenticada;
- exatamente uma busca sobre memoria vazia;
- resultado da busca sem handles;
- apresentacao das escolhas antes da primeira resposta humana;
- apresentacao completa da operacao antes da confirmacao;
- duas respostas humanas separadas na ordem congelada;
- exatamente uma chamada `memory_propose_create_page`, somente depois da confirmacao.

O runner nao persistiu a metrica parcial da sessao que falhou. Por isso modelo observado, turnos, custo, duracao, tool counts e conteudo das respostas sao desconhecidos e nao devem ser reconstruidos por suposicao.

Depois do aborto, antes do commit unico de T19, o runner foi endurecido para rejeitar explicitamente eventos `result.is_error` e para tornar os checks sanitizados de escolha/review menos permissivos. Essas mudancas nao foram executadas sob este experiment ID, nao alteram o source hash pre-run acima e nao autorizam repeticao. O commit final tambem acrescenta este report e atualiza o status do plano; por isso seu tree nao e apresentado como se fosse o source tree observado pelo run.

## Pares e gate

| Medida | Resultado |
|---|---|
| sessao A aceita | nao |
| pagina aplicada/publicada | nao |
| pares iniciados | 0/3 |
| pares validos | 0/3 |
| correctness comparavel | indisponivel |
| `task_duration_ms` comparavel | indisponivel |
| pares acima de 25% | 0/3 observados |
| decisao | `revisar o fluxo` |

Os criterios de 3/3 pares validos, ausencia de regressao e reducao de pelo menos 25% em 2/3 pares nao foram demonstrados. Ausencia de amostra nao e empate nem falha de valor; e falha operacional anterior ao experimento comparativo.

## Privacidade e cleanup

- O evidence root existe e esta vazio; nenhum stream, prompt resolvido, resposta ou payload foi persistido.
- O auth root tmpfs e o diretorio temporario do experimento foram removidos pelo cleanup do runner.
- Nao restou diretorio `gpm-bootstrap-value-auth.*` em `/dev/shm` nem `gpm-value-experiment-*` em `/tmp` na verificacao posterior.
- Credencial, config temporaria, state root, repositorio de memoria e clones da tarefa nao foram preservados.
- O ledger local ignorado pelo Git contem somente IDs, hashes, revision base, aprovacao e evidence path.

Como o aborto ocorreu antes do scan final de marcadores, nao existe contagem classificatoria de privacy markers para este run. O evidence root vazio e a ausencia dos roots temporarios sustentam somente o cleanup, nao uma auditoria do stream descartado.

## Comparacao com T10

T10 concluiu seis runs validos e mediu chamadas de exploracao; somente 1/3 pares atingiu seu limiar. T19 pre-registrou duracao da tarefa como metrica primaria, mas nao iniciou os pares. As amostras nao sao combinadas: T10 continua sendo evidencia de correcao sem efeito consistente de exploracao, e T19 registra uma falha operacional do novo bootstrap autenticado.

## Consequencia

T17 continua provando tecnicamente o bootstrap sem Markdown e T18 continua mostrando overhead local baixo. T19 nao prova que uma sessao Claude consegue completar o fluxo interativo autenticado sob o harness congelado, nem que a memoria reduz duracao. A extensao da Fase 2 termina com decisao **`revisar o fluxo`**, e o inicio da Fase 3 nao esta autorizado.

Qualquer nova tentativa exige novo experiment ID, protocolo, seed, ordem e aprovacao humana. Antes dela, um harness sintetico sem inferencia deve tornar observavel, sem texto de resposta, qual gate de fase da sessao A falhou e testar deterministicamente a maquina de tres turnos.
