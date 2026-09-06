# T8: Claude Adapter explicito

## Fixed point e seams

O fixed point de T8 e `ec239292c6a2cfbce78cca60e09cb933783c6534`. `internal/mcpserver` e o Claude Adapter: recebe a entrada estruturada de `memory_propose_replace`, reconstrui o JSON fechado do candidato e chama Intake Guard antes de inicializar Proposal Workflow ou criar o state root. Depois da aprovacao, chama `Workflow.Prepare` e retorna somente ID, revisao, target e contagens compactas do diff.

O servidor MCP recebe memory root, code root e state root explicitos. Ele continua transitorio sobre stdio e expoe somente busca, expansao e proposta; apply e publish permanecem comandos humanos da CLI. O material sob `integrations/claude-code` fornece um skill de invocacao exclusivamente manual, sem SessionStart, Stop, PostToolUse ou outro hook e sem fallback persistente quando o MCP estiver ausente.

Os filtros focados sao `go test ./internal/mcpserver -run 'TestMemoryProposeReplace|TestInvalidAndSensitive|TestClaudeAdapter|TestMissingClaude'` e `go test ./internal/app -run 'TestMCP'`. A matriz final e `make verify` seguida de `make verify-offline`.

## Contratos

- Cada chamada aceita cria no maximo uma proposta e nao executa apply nem Git.
- Uma rejeicao do Intake Guard retorna erro estruturado da tool, nao cria state root, spool ou retry e mantem a sessao MCP utilizavel.
- O resumo informa linhas adicionadas e removidas sem devolver replacement, transcript ou corpo de sessao.
- Um novo Workflow aberto pela CLI consegue revisar a proposta persistida.
- Ausencia ou falha do servidor fica visivel ao Claude Code; o skill nao repete a chamada nem grava o payload em outro lugar.
