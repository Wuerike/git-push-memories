# Manifesto local de evidencias da Fase 1

As evidencias brutas necessarias para auditar os relatorios foram preservadas em `.artifacts/phase-1/`. O diretorio e ignorado pelo Git porque contem bancos, logs, outputs extensos e fixtures sinteticos de privacidade. Este manifesto versionado identifica o conjunto local esperado; ele nao substitui um artifact store compartilhado.

Os diretorios completos de execucao, clones descartaveis, toolchains, caches e tentativas substituidas nao fazem parte do arquivo. Comandos, conclusoes e extratos necessarios para interpretar cada classificacao continuam nos relatorios versionados.

## Inventario

| Diretorio em `.artifacts/phase-1/` | Bytes | Entradas | SHA-256 deterministico |
|---|---:|---:|---|
| `20260904T211353Z-basic-memory-stage3-evidence` | 11.123.153 | 98 | `b5a65adda336f301353e6f42917dd0cf74f8831f5ccfbe81e0726201dadc43b1` |
| `20260904T212935Z-ai-memory-stage3-evidence` | 7.446.781 | 141 | `7084d27e599d834ac26f4ec22c3ffbc8f936accbc1ae850c361fa78c075d2d86` |
| `20260904T233117Z-ai-memory-stage4-evidence` | 9.255.510 | 193 | `4158ec34a589eaeb34dfe1223a801ea3a2d63a158a3749fd72d6453178527bfd` |
| `20260904T234427Z-palinode-stage4-evidence` | 2.292.051 | 165 | `9bb740bb694797942e240252b6e6cf8befcf1bbb69dca8ae192e8334673f95e5` |
| `20260904T234627Z-claude-bedrock-stage4-gate-evidence` | 28.526 | 22 | `a87f64880cf5916148187cfb2c92b195198926e6a3b923916add1b606ecc7c28` |
| `20260905T024115Z-agent-memory-stage5-evidence` | 55.072.851 | 443 | `b8dbf8c9d166fe8a72601c9004418b01cc0d2460a427b7cd108de427c626dff0` |
| `stage6-agent-memory` | 134.117 | 42 | `301c553a68e549fe6e7957735248854e176ca5241c217f07ad27efbe8bde1a97` |
| `stage6-ai-memory` | 1.443.142 | 240 | `e6e4e14c1d1a21b6596d54e9b7684a050dc7c127c0304354604f11cb07237769` |
| `stage6-claude-mem` | 773.577 | 52 | `ff17d197b21f970630d3958992702735213525f918b327aeaa418f1d4f980c0e` |

Total preservado: 87.569.708 bytes.

## Conteudo da Etapa 6

- `stage6-ai-memory`: streams e metricas, SQLite, logs, wiki e configuracoes isoladas de hooks/MCP; o `config.toml` bruto com material de autenticacao gerado nao foi copiado.
- `stage6-agent-memory`: clientes e respostas MCP, streams Claude, tempos, diff/review/apply e estado final `.agent-memory` com skill e `.mcp.json`.
- `stage6-claude-mem`: harness, provider local, payloads, streams, tempos, SQLite, logs, configuracoes sanitizadas e inventarios.

## Verificacao

O checksum de cada linha foi calculado a partir da raiz do repositorio:

```bash
tar --sort=name --mtime='UTC 1970-01-01' --owner=0 --group=0 --numeric-owner \
  -cf - ".artifacts/phase-1/<diretorio>" | sha256sum
```

As copias tambem foram comparadas com suas origens por `diff -qr` antes de qualquer limpeza de `/tmp`. Os diretórios de origem ainda não foram removidos.

Como `.artifacts/` nao e versionado, outro clone nao recebe esses arquivos. Antes de mover o trabalho para outra maquina ou apagar este workspace, exporte `.artifacts/phase-1/` para armazenamento duravel e verifique os checksums deste manifesto.
