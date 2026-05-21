# Ausgleichstage durch Wochenendarbeit

## Ziel

Mitarbeitende erhalten für jeden gearbeiteten Wochenend-Kalendertag einen Anspruch auf einen kompletten freien Ausgleichstag. Die am Wochenende geleisteten Stunden bleiben regulär im Überstundenkonto. Wenn der Ausgleichstag genommen wird, verbraucht er das normale Tagessoll aus dem Überstundenkonto.

Beispiel: Tim hat 10 Überstunden und arbeitet am Samstag 1 Stunde. Danach hat er 11 Überstunden und einen offenen Ausgleichstag-Anspruch. Nimmt Tim später einen Ausgleichstag bei einem Tagessoll von 8 Stunden, sinkt sein Überstundenkonto auf 3 Stunden und der Anspruch ist verbraucht.

## Fachmodell

Ein Ausgleichstag wird als eigene Abwesenheitsart geführt:

- API-Wert: `compensation_day`
- UI-Bezeichnung: `Ausgleichstag`
- zählt nicht als Urlaub
- reduziert keinen Urlaubsanspruch
- ist immer ganztägig

Zusätzlich wird eine eigene Anspruchstabelle eingeführt, z. B. `compensation_day_claims`.

Felder:

- `id`
- `user_id`
- `work_date`: gearbeiteter Samstag oder Sonntag
- `status`: `open`, `used`, `waived`
- `used_absence_id`: verweist auf die Ausgleichstag-Abwesenheit, wenn der Anspruch genutzt wurde
- `created_at`
- `updated_at`

Pro Mitarbeiter und Wochenenddatum darf es höchstens einen Anspruch geben.

## Anspruchsentstehung

Für jeden Samstag oder Sonntag entsteht genau ein Anspruch, sobald an diesem Kalendertag mindestens eine abgeschlossene, abrechenbare Arbeitszeit vorhanden ist.

Regeln:

- Samstag und Sonntag werden getrennt gezählt.
- Eine Stunde am Samstag reicht für einen ganzen Anspruch.
- Mehrere Einsätze am selben Samstag ergeben weiterhin nur einen Anspruch.
- Gearbeitete Wochenendstunden bleiben unverändert als Überstunden im Zeitkonto.

Wenn Wochenend-Arbeitszeit später entfernt oder so korrigiert wird, dass an diesem Tag keine abrechenbare Arbeitszeit mehr existiert, wird ein noch offener Anspruch automatisch entfernt. Bereits genutzte oder verzichtete Ansprüche bleiben historisch erhalten.

## Ausgleichstag Nehmen

Eine Abwesenheit vom Typ `compensation_day` darf nur erstellt werden, wenn mindestens ein offener Anspruch vorhanden ist.

Validierung:

- Datum ist ein regulärer Arbeitstag.
- Kein Samstag oder Sonntag.
- Kein Feiertag.
- Kein halber Tag.
- Es existiert mindestens ein offener Ausgleichstag-Anspruch.

Beim Speichern wird der älteste offene Anspruch des Mitarbeiters automatisch verwendet:

- Abwesenheit `compensation_day` wird angelegt.
- Anspruch wird auf `used` gesetzt.
- `used_absence_id` verweist auf die angelegte Abwesenheit.

Die Stundenberechnung behandelt den genommenen Ausgleichstag ohne zusätzliche Gutschrift. Dadurch fehlt an diesem Arbeitstag das volle Tagessoll und wird aus vorhandenen Überstunden bezahlt.

## Verzicht

Leitung oder Superadmin kann einen offenen Anspruch aktiv als `waived` markieren.

Der Verzicht verändert keine Überstunden. Er entfernt nur den Anspruch auf einen kompletten freien Ausgleichstag.

## API

Das Backend ergänzt die bestehende Abwesenheitslogik.

Neue oder erweiterte Fähigkeiten:

- Ansprüche pro Mitarbeiter listen, gefiltert nach Status.
- Offenen Anspruch als `waived` markieren.
- Beim Erstellen einer Abwesenheit vom Typ `compensation_day` die Anspruchs- und Datumsregeln validieren.
- Beim erfolgreichen Erstellen eines Ausgleichstags einen offenen Anspruch verbrauchen.
- Bei Änderungen an Wochenend-Arbeitszeiten offene Ansprüche synchronisieren.

Fehlermeldungen sollen sprechend sein, z. B.:

- `Für diesen Mitarbeiter ist kein offener Ausgleichstag-Anspruch vorhanden.`
- `Ein Ausgleichstag kann nur an regulären Arbeitstagen genommen werden.`
- `Halbe Ausgleichstage sind nicht möglich.`

## UI

Die Abwesenheiten-Ansicht erhält die neue Art `Ausgleichstag`.

Bei Auswahl von `Ausgleichstag` zeigt die UI:

- ob der ausgewählte Mitarbeiter offene Ansprüche hat
- wie viele offene Ansprüche vorhanden sind
- eine sprechende Fehlermeldung, wenn kein Anspruch vorhanden ist
- eine sprechende Fehlermeldung, wenn das gewählte Datum Wochenende oder Feiertag ist

In der Team-/Mitarbeiterübersicht sollte sichtbar sein, wie viele Ausgleichstag-Ansprüche offen sind. Genutzte und verzichtete Ansprüche können in einer Detailansicht nachvollziehbar bleiben.

## Tests

Mindestens folgende Fälle werden abgesichert:

- Eine Stunde Arbeit am Samstag erzeugt einen offenen Anspruch.
- Arbeit am Samstag und Sonntag erzeugt zwei offene Ansprüche.
- Mehrere Einsätze am selben Wochenendtag erzeugen nur einen Anspruch.
- Ausgleichstag ohne offenen Anspruch wird abgelehnt.
- Ausgleichstag am Wochenende wird abgelehnt.
- Ausgleichstag am Feiertag wird abgelehnt.
- Halber Ausgleichstag wird abgelehnt.
- Genommener Ausgleichstag reduziert den Überstundensaldo um das Tagessoll.
- Verzicht setzt den Anspruch auf `waived` und verändert keine Überstunden.
