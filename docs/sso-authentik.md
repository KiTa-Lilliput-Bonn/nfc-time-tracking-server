# SSO mit Authentik einrichten

Schritt-für-Schritt-Anleitung, um die NFC-Zeiterfassung an Authentik anzubinden. Danach melden sich Mitarbeitende mit ihrem Authentik-Konto an (Passwort, Passkey, zweiter Faktor, je nachdem, was in Authentik eingestellt ist). Allgemeines zur SSO-Anmeldung steht in [`sso.md`](sso.md).

In den Beispielen gilt:

| Was | Beispieladresse |
| --- | --- |
| Zeiterfassung | `https://zeit.example.de` |
| Authentik | `https://auth.example.de` |
| Slug der Anwendung in Authentik | `zeiterfassung` |

Die Adressen durch die eigenen ersetzen.

## Voraussetzungen

- Authentik läuft und ist unter seiner Adresse per **HTTPS** erreichbar.
- Die Zeiterfassung ist unter ihrer endgültigen Adresse erreichbar (Reverse Proxy oder Tunnel). Der Login über SSO funktioniert auch über `http://` im LAN, für den Zugriff von außen aber nur mit HTTPS einsetzen.
- Der **Server der Zeiterfassung** (bzw. ihr Docker-Container) muss Authentik unter **derselben Adresse** erreichen, die auch der Browser benutzt (`https://auth.example.de`). Die App prüft, dass der Aussteller in den Tokens genau zu dieser Adresse passt. Läuft beides auf demselben Host und klappt der Zugriff auf die eigene öffentliche Adresse nicht (Hairpin-NAT), im Router oder per `extra_hosts` in `docker-compose.yml` dafür sorgen, dass `auth.example.de` auflösbar ist.
- Die Uhrzeit auf beiden Servern stimmt (NTP). Tokens sind nur wenige Minuten gültig.

## 1. Benutzernamen abgleichen

Beim ersten SSO-Login verknüpft die App das Authentik-Konto über den **Benutzernamen** mit dem App-Benutzer. Groß- und Kleinschreibung spielen keine Rolle, sonst müssen die Namen gleich sein.

- In der Zeiterfassung unter **Mitarbeiter** bzw. **Benutzer (Admin)** nachsehen, welche Benutzernamen vergeben sind (z. B. `anna`, `b.mueller`).
- In Authentik unter **Directory → Users** die Konten mit genau diesen Benutzernamen anlegen bzw. umbenennen.

Konten werden **nicht** automatisch angelegt: Wer in der Zeiterfassung keinen Benutzer hat, kommt auch über SSO nicht hinein.

Optional, aber empfohlen: In Authentik eine Gruppe anlegen, z. B. `zeiterfassung`, und alle Mitarbeitenden hinzufügen. Damit wird in Schritt 2 gesteuert, wer die App überhaupt sehen darf.

## 2. Anwendung und Provider in Authentik anlegen

In der Authentik-Admin-Oberfläche **Applications → Applications → Create with Provider** (Assistent):

**Application**

- Name: `NFC Zeiterfassung`
- Slug: `zeiterfassung` (der Slug steckt später in der Issuer-Adresse)
- Launch URL (optional): `https://zeit.example.de` (dann erscheint die App im Authentik-Benutzerportal)

**Provider-Typ:** `OAuth2/OpenID Provider`

**Provider-Einstellungen**

- Authorization flow: `default-provider-authorization-implicit-consent` (keine extra Zustimmungsseite)
- Client type: **Confidential**
- Client ID und Client Secret: von Authentik erzeugt. **Beide notieren**, sie kommen in Schritt 3 in die Zeiterfassung.
- Redirect URIs/Origins: Modus **Strict**, Wert
  `https://zeit.example.de/api/v1/auth/oidc/callback`
- **Signing Key: ein Zertifikat auswählen**, z. B. `authentik Self-signed Certificate`.
  Wichtig: Ohne Signing Key signiert Authentik die Tokens mit dem Client Secret (HS256). Das nimmt die App nicht an, die Anmeldung schlägt dann fehl.
- Unter **Advanced protocol settings**:
  - Scopes: `openid`, `email`, `profile` (Standard, so lassen)
  - Subject mode: `Based on the User's hashed ID` (Standard, so lassen). Diese Kennung merkt sich die App dauerhaft. Später nicht mehr umstellen, sonst passen die gespeicherten Verknüpfungen nicht mehr (siehe „Verknüpfung lösen“ unten).

**Bindings** (optional, empfohlen): Gruppe `zeiterfassung` binden. Dann dürfen sich nur Mitglieder dieser Gruppe an der App anmelden.

Nach dem Speichern die Anwendung öffnen und unter **Provider → Overview** den Wert **OpenID Configuration Issuer** kopieren, z. B.

```
https://auth.example.de/application/o/zeiterfassung/
```

Der Schrägstrich am Ende gehört dazu.

## 3. Zeiterfassung konfigurieren

### Mit Docker (`docker-compose.yml`)

Beim Dienst `nfc-time-tracking` unter `environment` ergänzen:

```yaml
    environment:
      NFC_BACKUP_TARGET_PATH: /backup
      NFC_OIDC_ENABLED: "true"
      NFC_OIDC_ISSUER: "https://auth.example.de/application/o/zeiterfassung/"
      NFC_OIDC_CLIENT_ID: "<Client ID aus Authentik>"
      NFC_OIDC_CLIENT_SECRET: "<Client Secret aus Authentik>"
      NFC_OIDC_REDIRECT_URL: "https://zeit.example.de/api/v1/auth/oidc/callback"
      NFC_OIDC_PASSWORD_LOGIN: "all"
      # optional:
      # NFC_OIDC_BUTTON_LABEL: "Mit Kita-Konto anmelden"
```

Das Secret lieber nicht direkt in die Compose-Datei schreiben, sondern in eine `.env`-Datei neben `docker-compose.yml` (`NFC_OIDC_CLIENT_SECRET=...`) und in der Compose-Datei `NFC_OIDC_CLIENT_SECRET: "${NFC_OIDC_CLIENT_SECRET}"` verwenden.

Danach neu starten:

```bash
docker compose up -d
```

### Ohne Docker (`config.yaml`)

```yaml
auth:
  oidc:
    enabled: true
    issuer: "https://auth.example.de/application/o/zeiterfassung/"
    client_id: "<Client ID aus Authentik>"
    client_secret: "<Client Secret aus Authentik>"
    redirect_url: "https://zeit.example.de/api/v1/auth/oidc/callback"
    password_login: "all"
```

Server neu starten.

### Kontrolle

Im Server-Log (`docker compose logs nfc-time-tracking` bzw. `data/server.log`) steht nach dem Start:

```
SSO (OIDC) aktiv: issuer=https://auth.example.de/application/o/zeiterfassung/, password_login=all
```

Fehlt eine Pflichtangabe, startet der Server nicht und nennt im Log, was fehlt.

## 4. Testen

1. In einem privaten Browserfenster `https://zeit.example.de/login` öffnen. Dort erscheint **„Mit SSO anmelden“** über dem Passwortformular.
2. Klicken, bei Authentik mit einem Testkonto anmelden (z. B. `anna`).
3. Man landet im Dashboard der Zeiterfassung als Anna. Damit ist das Konto verknüpft.
4. Als Leitung oder Superadmin unter **Mitarbeiter → Anna → Einstellungen** steht jetzt **SSO: verknüpft**. In **Benutzer (Admin)** gibt es dafür die Spalte „SSO“.
5. **Abmelden** in der Zeiterfassung meldet auch bei Authentik ab (Authentik zeigt dabei seine Abmeldeseite).

Danach meldet sich jede Person einmal über SSO an, damit alle Konten verknüpft sind.

## 5. Passwort-Login einschränken

Solange `password_login` auf `all` steht, geht beides: SSO und das bisherige Passwort. Sind alle Mitarbeitenden verknüpft, umstellen auf:

```yaml
      NFC_OIDC_PASSWORD_LOGIN: "admins"
```

Dann melden sich Mitarbeitende nur noch über Authentik an. Leitung und Superadmin können sich zusätzlich weiter mit Passwort anmelden (auf der Login-Seite unter „Mit Passwort anmelden (Leitung)“). Das ist der Notzugang, falls Authentik einmal nicht läuft. Deshalb **nicht** `none` einstellen, solange es keinen anderen Weg in die App gibt.

Beim Wechsel einer Person ins Team: in der Zeiterfassung den Benutzer anlegen, in Authentik ein Konto mit **demselben Benutzernamen** anlegen (und in die Gruppe `zeiterfassung` aufnehmen). Das Einmalpasswort aus der Zeiterfassung wird dann nicht gebraucht.

Beim Ausscheiden: in der Zeiterfassung den Benutzer **deaktivieren**. Das wirkt sofort, auch für eine noch offene Sitzung. Zusätzlich das Authentik-Konto deaktivieren.

## Verknüpfung lösen

Wurde ein Authentik-Konto falsch zugeordnet oder das Konto in Authentik neu angelegt, meldet die App „Das Konto ist bereits mit einem anderen SSO-Konto verknüpft“. Dann:

- In der Zeiterfassung unter **Mitarbeiter → Person → Einstellungen** bei „SSO“ auf **Lösen** klicken (oder in **Benutzer (Admin)** → Bearbeiten → **Verknüpfung lösen**).
- Die Person meldet sich erneut über SSO an; die App verknüpft wieder über den Benutzernamen.

Das Verknüpfen und Lösen steht im Audit-Log.

## Fehlersuche

Die Login-Seite zeigt eine kurze Meldung, Details stehen im Server-Log (Zeilen mit `oidc`).

| Meldung auf der Login-Seite / im Log | Ursache und Lösung |
| --- | --- |
| Authentik zeigt „Redirect URI Error“ | Die Redirect-URI in Authentik stimmt nicht genau mit `NFC_OIDC_REDIRECT_URL` überein (Schreibweise, `https`, kein Schrägstrich am Ende). |
| „Anmeldung über SSO ist fehlgeschlagen“, Log: `oidc discovery: …` | Issuer falsch (Slug, Schrägstrich am Ende) oder der Server erreicht Authentik nicht unter dieser Adresse (DNS, Hairpin-NAT, Zertifikat). Test vom Server aus: `curl https://auth.example.de/application/o/zeiterfassung/.well-known/openid-configuration` |
| Log: `oidc id_token: oidc: malformed jwt: … unexpected signature algorithm "HS256"` | Im Authentik-Provider ist kein **Signing Key** ausgewählt. Zertifikat auswählen, speichern, erneut anmelden. |
| Log: `oidc token exchange: … invalid_client` | Client ID oder Client Secret falsch übernommen. |
| Log: `oidc id_token: … token is expired` oder `… before the nbf (not before) time` | Uhrzeit auf einem der Server falsch. NTP prüfen. |
| „Zu diesem SSO-Konto gibt es keinen Benutzer“, Log: `kein App-Benutzer "…"` | Der Authentik-Benutzername (steht im Log in Anführungszeichen) passt zu keinem Benutzer der Zeiterfassung. Benutzernamen angleichen. |
| „bereits mit einem anderen SSO-Konto verknüpft“ | Siehe „Verknüpfung lösen“. |
| „Ihr Konto in der Zeiterfassung ist deaktiviert“ | Benutzer in der Zeiterfassung wieder aktivieren, falls gewollt. |
| „Die Anmeldung beim SSO-Anbieter wurde abgebrochen“ | Die Person hat bei Authentik abgebrochen oder darf die Anwendung laut Binding nicht nutzen (Gruppe prüfen). |
| „Anmeldung mit Passwort ist für dieses Konto abgeschaltet“ | `password_login` steht auf `admins` oder `none`. Über SSO anmelden. |
