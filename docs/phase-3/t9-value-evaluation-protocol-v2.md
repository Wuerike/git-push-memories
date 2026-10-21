# T9: protocolo de valor v2 para o ambiente vigente

> Registro historico. T9 e T10 foram retiradas do plano por decisao do usuario em 2026-09-11. O [plano revisado](plan.md#revisao-de-escopo-em-2026-09-11) prioriza o uso da ferramenta; a execucao descrita abaixo pertence a tentativa encerrada.

Pre-registro preparado em 2026-09-11, antes de qualquer inferencia autenticada de T10.

## Identidade e mudanca

- Experiment ID: `phase3-curated-memory-value-v2`.
- Configuracao do experimento: `testdata/phase-3/value-experiment/protocol.json`.
- SHA-256: `41936567f2489a5e5336ebc348c3357e90e247df12bf0070d6dc86cfee7b9992`.
- Produto avaliado: `e92a96b6bc4c6f2f169b8b8e96a419c9d9a1b9b2`.
- Git exigido: `2.55.0`.

O experimento esta retirado; o hash dos assets de producao fixado no protocolo foi atualizado em 2026-10-20, depois que a skill `/curate` mudou, e por isso o SHA-256 acima difere do original.

V2 sucede o [protocolo v1](t9-value-evaluation-protocol.md) preservado no commit `938f38fa4a208cf5d277c3d5fb4f41265a69e1f8`, cujo SHA-256 e `964e5a988af33b10f135a26258e66aeec33cf9b0e5694304434f70ab99bc149f`.

A divergencia entre Git `2.54.0` pre-registrado e `2.55.0` instalado encerrou as tentativas v1 antes da criacao do ledger e antes de inferencia. O usuario autorizou preparar o novo pre-registro e executar T10. As unicas diferencas no JSON sao `experiment_id` e `environment.git_version`.

Todas as demais regras de v1 se aplicam integralmente, incluindo corpus, produto, ferramentas, isolamento, revisao humana, metricas, limiares, evidencia e decisao. O seed `gpm-phase3-value-v1` e seus assignments sao preservados: nenhuma sessao autenticada ou outcome foi observado nas tentativas anteriores. `TestPhase3ValueV2ChangesOnlyRuntimeIdentity` compara o JSON ativo com os bytes commitados de v1 e limita a mudanca aos dois campos acima.

## Execucao autorizada na tentativa v2

Depois de commitar e verificar este pre-registro:

```bash
tests/experiment/run-phase-3-value.sh --approved 20260911-value-v2
```

Evidence root: `.artifacts/phase-3/20260911-value-v2`. Limite global: USD 25,00 de consumo reportado do modelo, com os limites por sessao herdados de v1. A attestation de cegueira e as confirmacoes de captura e curadoria sao feitas pelo revisor humano, conforme T9.

Os resultados efetivos e bloqueios estao registrados no [acompanhamento historico de T10](t10-value-evaluation-report.md).
