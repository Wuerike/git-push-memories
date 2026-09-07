# Protocolo de perfil do overhead da Fase 2

Status: pre-registrado antes da coleta T18.

## Hipotese e referencia

O overhead local controlado pelo produto nao explica sozinho a latencia dos treatments de T10. A referencia e a mediana preexistente dos tres treatments de T10: `214943 ms` (`232233`, `214943`, `54590`). O limiar de participacao e 25% dessa referencia, `53735 ms`.

Uma otimizacao e obrigatoria somente se:

- a mediana da soma local `mcp_ready + search + expand + prepare + show + apply + publish` exceder `53735 ms`; ou
- o p95 de qualquer uma dessas etapas exceder `2000 ms`.

Os limiares, etapas e metodo nao mudam depois da primeira repeticao medida.

## Procedimento

1. Executar uma repeticao de warmup excluida.
2. Executar dez repeticoes independentes do E2E de bootstrap, cada uma com roots e processos novos.
3. Extrair somente nome da etapa, exit code e duracao monotonicamente observada pelo harness.
4. Calcular p50 e p95 por nearest-rank sobre os dez valores e p50 da soma local por run.
5. Se algum limiar disparar, corrigir somente a etapa dominante e repetir o mesmo protocolo; caso contrario, registrar `optimization_required=false` e nao alterar runtime.

## Privacidade e evidencia

Os manifests intermediarios ficam em diretorio temporario e sao removidos ao final. A evidencia preservada contem somente revisao, ambiente, hashes dos sources/protocolo, limiares e agregados de duracao. Payload, Markdown, diff, prompt, resposta, transcript, credencial e state root nao sao preservados.
