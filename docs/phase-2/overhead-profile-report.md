# Resultado do perfil de overhead da Fase 2

## Decisao

O run classificatorio `20260907T021520Z-overhead-profile` executou um warmup excluido e dez repeticoes medidas conforme o [protocolo pre-registrado](overhead-profile-protocol.md). O runner gravou receipt, checksum e manifest read-only dos sources executaveis antes do warmup. Isso prova consistencia interna e ordem do procedimento, mas nao fornece timestamp externo imutavel: a restricao de um unico commit para T18 impede ancorar o protocolo em commit anterior. A ordem da execucao tambem esta registrada no log desta sessao. Resultado: `optimization_required=false`.

O total local p50 foi `158 ms` e o p95 foi `167 ms`. O limiar total pre-registrado era `53735 ms`, 25% da mediana de `214943 ms` dos treatments T10. Nenhuma etapa teve p95 acima do limiar de `2000 ms`.

| Etapa | p50 | p95 |
|---|---:|---:|
| MCP ready | 9 ms | 10 ms |
| search | 0 ms | 1 ms |
| expand | 4 ms | 5 ms |
| prepare create page | 21 ms | 28 ms |
| show | 6 ms | 8 ms |
| apply | 37 ms | 42 ms |
| publish | 78 ms | 84 ms |

`search` ficou abaixo da resolucao de um milissegundo do harness. Publish foi a maior etapa local, mas seu p95 representou 4,1% do limiar por etapa e nao autoriza mudanca de runtime, daemon ou cache.

## Interpretacao

A medicao separa o overhead local controlado pelo produto da espera humana e da inferencia autenticada: somente as sete etapas locais entram nos `158 ms`; espera humana nao foi simulada e inferencia nao foi executada. A diferenca de ordens de grandeza para T10 indica que seus treatments lentos nao podem ser atribuidos somente ao startup, retrieval, proposal, apply ou publish locais medidos aqui.

Conforme o protocolo, nenhuma otimizacao foi implementada. T19 deve manter o fluxo revisado e medir novamente a duracao autenticada sem alterar limiares depois dos runs.

## Evidencia

- ambiente: Linux amd64, Go 1.25.12, 12 CPUs logicas, 12th Gen Intel Core i7-1265U;
- corpus: zero Markdown inicial, um arquivo de codigo e 34 bytes;
- comandos: sete comandos sanitizados registrados no summary;
- fixed point: `d90231a567a1d53f70498068a6c5bccc1dbb7ee6`;
- protocol SHA-256: `7a2db4588a4d2e53f021a026abfbe3be489b754c56666e3041897d4dc47745f1`;
- evaluated executable sources SHA-256: `e298a2894b2a36d0030faff853395a893d4234f189587f51dc2ee168f77d5d8a`;
- preregistration receipt SHA-256: `0ba49d4e5a85ff7c1e6c9b5182471d96b55f04677324988f596838625679d915`;
- summary SHA-256: `7c6204dd7de01af4889e9443662a85382e7e4c7cba1723c1a134c9d6f689ea57`;
- evidencia local: `.artifacts/phase-2/20260907T021520Z-overhead-profile`.

Os manifests e logs intermediarios foram removidos com o diretorio temporario. A evidencia preservada declara `payloads_persisted=false` e contem somente agregados e metadados permitidos.
