# Dashboard Team‑Uebersicht (Leitung) - Design

## Kontext
Die Leitung soll auf dem Dashboard eine eigene Uebersicht bekommen, die alle aktiven Mitarbeitenden zeigt.
Aktuell zeigt das Dashboard nur eine einfache Team‑Tabelle (Name/Rolle/Aktiv).

## Ziele
- Eigener Abschnitt/Karte auf dem Leitung‑Dashboard.
- Tabelle mit sortierbarer Uebersicht und Textsuche (Name).
- Pro Mitarbeiter:
  - Stundensaldo gesamt (korrigiert + manuell)
  - Urlaub geplant (ab morgen, aktuelles Jahr)
  - Urlaub frei
  - Rest gesamt (inkl. Uebertrag, Tooltip mit Aufschluesselung)
- Schnelle Ladezeit bei ~20 Mitarbeitenden.
- Stichtag fuer Stundensaldo ist waehbar (nicht persistent) mit Default aus Settings.

## Nicht‑Ziele
- Vollstaendige Historie/Detailansicht pro Mitarbeiter.
- Echtzeit‑Live‑Anwesenheit.
- Cross‑Year Urlaubsplanung (nur aktuelles Jahr).

## Definitionen
- **Stundensaldo gesamt:** Saldo *seit* Stichtag bis **gestern** (letzter kompletter Tag), inkl. Korrekturen und manueller Zeiten.
  **Saldo-Algorithmus:** identisch mit Export‑Saldo (taegliche Soll/Ist‑Logik inkl. Feiertage/Schliessungen) – nicht die aktuelle Monats‑Balance API.
- **Stichtag:** Standardwert wird serverseitig vorgegeben; in der Uebersicht ueberschreibbar (nur fuer die Session).
- **Geplanter Urlaub:** Urlaubstage mit `absence_type = vacation` und Datum **> heute** im **aktuellen Jahr**.
- **Urlaub frei:** Resttage minus geplante Urlaubstage.
- **Halbe Tage:** 0,5 Tage.

## Ansatz (empfohlen)
Server‑seitige Aggregation in einem einzigen Endpoint (schnell, konsistent).

### Endpoint
`GET /api/v1/dashboard/team-overview?as_of=YYYY-MM-DD&vacation_year=YYYY`

**Antwort (Envelope):**
```json
{
  "as_of": "2026-03-15",
  "vacation_year": 2026,
  "rows": [
    {
      "id": 123,
      "display_name": "Anna M.",
      "hours_balance": 12.5,
      "vacation_planned": 3.0,
      "vacation_free": 10.0,
      "vacation_remaining_total": 13.0,
      "vacation_carryover": 2.0,
      "vacation_entitlement": 25.0,
      "vacation_taken": 12.0
    }
  ]
}
```

## UI‑Design
- **Eigener Abschnitt** im Dashboard unterhalb der Karten.
- **Tabelle** (kompakt) mit Scrollbar:
  - Name
  - Stundensaldo (gesamt)
  - Urlaub geplant
  - Urlaub frei
  - Rest gesamt (Tooltip: Rest gesamt / Uebertrag / Anspruch / Genommen)
- **Default‑Sortierung:** Name A–Z.
- **Filter:** Textsuche nach Name.
- **Rollenfilter:** Nur aktive Mitarbeitende (Leitung optional anzeigen, siehe Annahmen).
- **Stichtag‑Picker** innerhalb der Karte (nur fuer Leitung; nicht gespeichert).

## Datenfluss
1. Dashboard laedt Standarddaten.
2. Leitung bekommt zusaetzlich `team-overview` (as_of, vacation_year).
3. UI rendert Tabelle; Aenderung des Stichtags triggert Reload.

## Fehlerbehandlung
- Backend: 400 bei ungueltigem `as_of` oder `vacation_year`.
- UI: dezente Fehlermeldung im Kartenbereich, restliches Dashboard bleibt sichtbar.

## Tests
- Store/Service‑Tests fuer Aggregation:
  - Stichtag‑Logik (seit Stichtag bis gestern)
  - Urlaub geplant/frei inkl. halbe Tage
  - Uebertrag in Rest gesamt
- Handler‑Test fuer Rollenrechte und Response‑Shape.
- Optional UI‑Test fuer Render + Sort/Filter.

## Akzeptanzkriterien
- Leitung sieht eigenen Abschnitt mit Tabelle.
- Stundensaldo und Urlaubssalden pro Mitarbeiter werden korrekt berechnet.
- Stichtag ist in der Uebersicht waehbar und beeinflusst die Werte sofort.
- Tabelle ist sortierbar (Name default) und filterbar (Textsuche).
- Ladezeit ist fuer ~20 Mitarbeiter subjektiv schnell.

## Offene Annahmen / Entscheidungsbedarf
- **Settings‑Key:** Neuer Setting‑Key `dashboard.team_overview.as_of_default` (YYYY‑MM‑DD). Falls nicht gesetzt: default = 01.01. des aktuellen Jahres (lokal).
- **Rollenfilter:** Standard: nur `active = true` und `role != superadmin`. Leitung selbst anzeigen?
- **Zeitbasis:** "Heute/gestern" in lokaler Server‑Zeit (nicht UTC).
- **Stichtag‑Grenzfall:** Wenn `as_of` > gestern -> leere Spanne (Saldo 0).
- **Korrekturen:** Korrigierte Zeiten ersetzen Original‑Stempel je `work_period_id` (neu im Service).
- **Urlaub/Uebertrag:** Aktuell kein Carryover im Modell; kurzfristig `vacation_carryover = 0` und Tooltip zeigt Entitlement/Taken/Remaining.
- **Urlaub frei Formel:** `vacation_free = vacation_remaining_total - vacation_planned`.
