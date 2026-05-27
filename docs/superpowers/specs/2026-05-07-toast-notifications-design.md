# Toast Notifications (PrimeVue) – 10 Sekunden, oben mittig

## Kontext
Im Web-Frontend werden PrimeVue Toasts über `useToast()` und `toast.add(...)` genutzt. Aktuell setzen einzelne Views unterschiedliche `life`-Werte (z. B. 3000–9000 ms) oder lassen `life` weg (Default).

## Ziel
- **Alle Toast Notifications** (success/info/warn/error) werden künftig **10 Sekunden** angezeigt.
- Anzeigeort ist **oben mittig** (`top-center`), entsprechend der globalen Toast-Instanz in `web/src/App.vue`.

## Nicht-Ziele
- Keine Änderung am Styling/Theme der Toasts.
- Keine Änderung an Texten/Severities oder Business-Logik.
- Keine Einführung eines neuen Notification-Systems.

## Design / Entscheidung
- PrimeVue Toast bleibt global in `web/src/App.vue` eingebunden und **positioniert sich oben mittig** (`top-center`).
- Um die Vorgabe „alle 10 Sekunden“ zuverlässig zu erfüllen, wird `life` für jede Toast-Nachricht auf **10000 ms** vereinheitlicht.

## Umsetzungsskizze
- Codebase-weite Suche nach `toast.add(`.
- Für jede Toast-Nachricht:
  - Wenn `life` existiert: auf `10000` setzen.
  - Wenn `life` fehlt: `life: 10000` ergänzen.

## Akzeptanzkriterien
- Jeder ausgelöste Toast bleibt **ca. 10 Sekunden** sichtbar.
- Es gibt keine Stellen mehr, die Toasts mit `life` ≠ 10000 erzeugen.
- Alle Toasts erscheinen **oben mittig** am Bildschirm.
