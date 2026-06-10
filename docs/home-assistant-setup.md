# Configuração Home Assistant — Bambu Lab + FilaBridge

Este guia explica como conectar impressoras **Bambu Lab** ao FilaBridge via **Home Assistant** e a integração **ha-bambulab**, replicando o fluxo do SpoolmanSync dentro do FilaBridge.

## Pré-requisitos

- [Spoolman](https://github.com/Donkie/Spoolman) acessível na rede
- [Home Assistant](https://www.home-assistant.io/) rodando
- [HACS](https://hacs.xyz/) instalado no HA
- Integração **[ha-bambulab](https://github.com/greghesp/ha-bambulab)** instalada via HACS
- Impressora Bambu adicionada no Home Assistant (LAN ou Cloud)
- FilaBridge acessível pelo HA na rede local

## 1. Instalar ha-bambulab no Home Assistant

1. Abra HACS → Integrações → pesquise **Bambu Lab**
2. Instale **ha-bambulab**
3. Reinicie o Home Assistant
4. Vá em **Configurações → Dispositivos e serviços → Adicionar integração → Bambu Lab**
5. Siga o assistente (LAN ou conta Bambu Cloud)
6. Confirme que aparecem entidades como:
   - `sensor.<prefix>_print_status`
   - `sensor.<prefix>_tray_1` … `tray_4`
   - `sensor.<prefix>_external_spool` (se aplicável)

## 2. Configurar FilaBridge

1. Abra FilaBridge → **Settings → Basic Configuration**
2. Na seção **Home Assistant**:
   - **HA URL**: `http://IP_DO_HA:8123`
   - **HA Token**: crie um Long-Lived Access Token em HA → Perfil → Tokens de acesso
   - **FilaBridge Public URL**: URL que o HA alcança, ex: `http://192.168.1.20:5000`
3. Clique **Test Connection** (usa os valores do formulário, mesmo antes de salvar)
4. Clique **Save HA Settings** para persistir

> **Importante:** Se o HA estiver em outra máquina, não use `localhost` na URL pública do FilaBridge.

## 3. Registrar impressora Bambu no FilaBridge

1. **Settings → Printers → Add Bambu Lab (HA)**
2. Selecione a impressora descoberta
3. A impressora aparece no dashboard **Bambu Lab Printers**

## 4. Gerar e instalar automações no HA

1. No dashboard ou em Settings → Printers, clique **Generate HA Config**
2. Salve o arquivo YAML baixado em `config/packages/filabridge_<prefix>.yaml` no Home Assistant  
   O nome do arquivo é **sempre em minúsculas** (ex.: `filabridge_03919c461204338.yaml`, não `filabridge_03919C461204338.yaml`)
3. Em `configuration.yaml`, garanta:

```yaml
homeassistant:
  packages: !include_dir_named packages
```

4. **Reinicie o Home Assistant** (obrigatório após utility_meter e template sensors)
5. No FilaBridge, clique **Validar HA** na impressora Bambu (ou confira manualmente em **Ferramentas de desenvolvedor → Estados**):
   - `sensor.filabridge_<prefix>_filament_usage`
   - `sensor.filabridge_<prefix>_filament_usage_meter` — **obrigatório**; sem ele, `utility_meter.calibrate` fica desconhecida
   - `input_number.filabridge_<prefix>_last_tray`
   - `sensor.filabridge_<prefix>_active_tray`

## 5. Mapear bobinas (Spoolman)

### Pela interface

No dashboard **Bambu Lab Printers**, use o dropdown em cada slot AMS para atribuir uma bobina do Spoolman.

### Por NFC (fluxo FilaBridge)

1. Gere tag NFC do **spool** na aba NFC
2. Gere tag NFC do **slot AMS** (seção AMS Slots)
3. Escaneie: primeiro o spool, depois o slot
4. O FilaBridge grava `extra.active_tray` no Spoolman com o `unique_id` da bandeja HA

Formato da location AMS:

```
{Nome da Impressora} - AMS 1 Slot 2
{Nome da Impressora} - External Spool
```

## 6. Como funciona o tracking automático

| Evento HA | Webhook FilaBridge | Ação |
|-----------|-------------------|------|
| Fim de impressão / troca de bandeja | `spool_usage` | Deduz peso do spool atribuído à bandeja ativa |
| Troca física de bobina (RFID) | `tray_change` | Auto-atribui spool pelo `extra.tag` aprendido |
| Bandeja vazia (`name=Empty`) | `tray_change` | Desatribui spool da bandeja |

O mapeamento bobina ↔ bandeja fica no Spoolman em `extra.active_tray` (valor = `unique_id` da entidade HA).

## 7. Testar

1. Atribua uma bobina a um slot AMS
2. Inicie uma impressão curta
3. Ao terminar, verifique no Spoolman se o peso foi deduzido
4. Troque uma bobina com tag RFID Bambu — deve reatribuir automaticamente

### Teste manual pelo Home Assistant (sem imprimir)

Os `rest_command` do package FilaBridge são **fixos** (não incluem o prefix da impressora):

| Serviço | Uso |
|---------|-----|
| `rest_command.filabridge_update_spool` | Simular débito de filamento |
| `rest_command.filabridge_tray_change` | Simular troca de bobina |

**Ferramentas de desenvolvedor → Ações** — exemplo para debitar 5,5 g do slot 3:

- **Ação:** `rest_command.filabridge_update_spool`
- **Dados YAML** (ajuste o `entity_id` da bandeja):

```yaml
filament_name: "Teste HA"
filament_material: "PLA"
filament_tray_uuid: ""
filament_used_weight: "5.5"
filament_color: "#FFFFFF"
filament_active_tray_id: "sensor.bambu_lab_a1_ams_tray_3"
```

Script reutilizável (editor de script do HA — **sem** chave no topo, comece em `alias:`):

```yaml
alias: "Teste FilaBridge - debitar Slot 3"
icon: mdi:printer-3d-nozzle
mode: single
sequence:
  - action: rest_command.filabridge_update_spool
    data:
      filament_name: "Teste HA"
      filament_material: "PLA"
      filament_tray_uuid: ""
      filament_used_weight: "5.5"
      filament_color: "#FFFFFF"
      filament_active_tray_id: "sensor.bambu_lab_a1_ams_tray_3"
```

Entidades com prefix da impressora (ex. `03919c461204338`): `sensor.filabridge_<prefix>_filament_usage_meter`, `input_number.filabridge_<prefix>_last_tray` — **não** confundir com os nomes dos `rest_command`.

## Troubleshooting

### `utility_meter.calibrate` — ação desconhecida

A automação `filabridge_update_spool_*` usa `utility_meter.calibrate` para zerar o acumulador após debitar filamento no Spoolman (mesma lógica do SpoolmanSync). **Não remova essa ação** — ela só aparece como desconhecida quando o utility meter não foi carregado no HA.

| Sintoma | Causa | Correção |
|---------|-------|----------|
| Só existe `filament_usage`, não `filament_usage_meter` | Package incompleto ou sem reinício do HA | Baixe **HA Config** de novo, substitua o arquivo inteiro em `packages/`, reinicie o HA |
| Aviso ao iniciar impressão | Automação inválida enquanto o meter não existe | Após corrigir o package, o aviso some sozinho |
| Package antigo com `cycle: none` | Valor inválido no `utility_meter` (bug corrigido no SpoolmanSync v1.2.0) | Regenere o YAML no FilaBridge (versão atual omite `cycle`) |

Checklist pós-reinício: use **Validar HA** no FilaBridge ou confirme as 4 entidades listadas na seção 4.

### Webhook não chega ao FilaBridge

- Teste do HA: `curl -X POST http://IP_FILABRIDGE:5000/api/webhook -H "Content-Type: application/json" -d '{"event":"spool_usage","active_tray_id":"test","used_weight":0}'`
- Confirme **FilaBridge Public URL** com IP da rede, não `localhost`
- Verifique firewall entre HA e FilaBridge

### Nenhuma impressora descoberta

- Confirme ha-bambulab instalado e impressora visível no HA
- Token HA válido com permissões de leitura
- Teste conexão em Settings

### Peso não deduzido

- Verifique automações `filabridge_update_spool_*` ativas no HA
- Confirme bobina atribuída ao slot (`extra.active_tray` no Spoolman)
- Veja logs do HA em **Configurações → Sistema → Logs**

A automação **não chama o webhook durante o print** — só no **fim** (`print_status` → `finish`/`idle`) ou na **troca de bandeja**. Durante a impressão, monitore se estes sensores sobem:

| Sensor | O que deve acontecer |
|--------|----------------------|
| `sensor.filabridge_<prefix>_filament_usage` | Sobe com o progresso |
| `sensor.filabridge_<prefix>_filament_usage_meter` | Acumula gramas |
| `sensor.bambu_lab_a1_print_weight` | > 0 (peso total do job) |
| `sensor.bambu_lab_a1_print_progress` | Sobe de 0 → 100 |
| `sensor.filabridge_<prefix>_active_tray` | Mostra o slot ativo (ex. `13` = AMS slot 3) |

Se `print_weight` ficar **0** o tempo todo, o meter nunca acumula e o débito é pulado (`tray_weight >= 0.01`).

No fim do print, a automação só debita se `print_status` vier de `running`/`pause`/etc. → `finish`/`idle`. Regenerar o package YAML no FilaBridge corrige `last_tray` desatualizado e o trigger `print_start` (grava o slot ao iniciar).

### RFID não faz auto-assign

- Só funciona com bobinas Bambu com tag RFID válida
- Spools sem RFID reportam `tray_uuid` só zeros — ignorados pelo FilaBridge
- O `extra.tag` é aprendido no primeiro `spool_usage` com RFID válido

### Coexistência com Moonraker (Snapmaker)

Impressoras Moonraker continuam com polling G-code. Impressoras Bambu usam apenas webhooks HA — não há conflito.
