# Bootstrap explícito de memória sem Markdown manual

Status: accepted

A Fase 2 deve conseguir criar a primeira página de memória em um Git worktree local já inicializado e com `HEAD`, mesmo quando ele não contém arquivos Markdown. O produto expõe operações estreitas de criação de página e seção, renderiza schema e anchors deterministicamente e mantém proposta, revisão humana, apply humano e publicação humana como etapas distintas; não expõe um changeset genérico nem exige que a pessoa escreva Markdown ou IDs internos.

## Considered Options

- Reaproveitar `replace_section_content`: rejeitado porque depende de página e anchor preexistentes e torna impossível o primeiro registro.
- Aceitar Markdown arbitrário em `put_page`: rejeitado porque transfere invariantes de schema, identidade e segurança para o cliente.
- Introduzir um changeset genérico: rejeitado nesta fase porque amplia atomicidade, recuperação e validação antes de existir necessidade comprovada.
- Inicializar ou clonar o repositório Git: adiado; o boundary atual começa em roots locais explícitos, distintos e já inicializados.

## Consequences

- `create_page`, `create_section` e `replace_section_content` são operações explícitas com validação própria.
- Para `create_page`, ausência do target em `HEAD`, index e worktree é parte do baseline e deve ser persistida em proposta e receipt.
- IDs são derivados deterministicamente de nomes validados; clientes fornecem intenção e conteúdo, não anchors.
- O bootstrap inicial cria página na raiz do memory root. Taxonomia, movimentação e criação em diretórios aninhados continuam na Fase 3.
