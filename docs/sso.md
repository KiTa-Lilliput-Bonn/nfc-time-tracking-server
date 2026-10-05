# Anmeldung über SSO (OpenID Connect)

Die Zeiterfassung kann die Anmeldung an einen OpenID-Connect-Provider abgeben, z. B. **Authentik**, **Authelia** oder **Pocket ID**. Der Provider prüft Passwort, Passkey oder zweiten Faktor; danach stellt die App wie bisher ihr eigenes Token aus. Rollen (Mitarbeiter, Leitung, Superadmin) und alle Daten bleiben in der App.

## Ablauf

1. Auf der Login-Seite erscheint der Button **„Mit SSO anmelden“**.
2. Die App leitet zum Provider weiter (Authorization Code Flow mit PKCE, `state` und `nonce`).
3. Nach der Anmeldung ordnet die App das SSO-Konto einem App-Benutzer zu:
   - Ist das SSO-Konto schon verknüpft, zählt nur die feste Kennung des Providers (`sub`). Spätere Umbenennungen beim Provider sind egal.
   - Beim **ersten** SSO-Login wird automatisch über den **Benutzernamen** verknüpft (Claim `preferred_username`, Groß-/Kleinschreibung egal). Die Benutzernamen müssen also beim Provider und in der App gleich sein.
   - Es werden **keine Konten automatisch angelegt**. Gibt es keinen passenden App-Benutzer, ist das Konto deaktiviert oder ist der App-Benutzer schon mit einem anderen SSO-Konto verknüpft, wird die Anmeldung mit einer Meldung abgelehnt.
4. In der Benutzer- und Mitarbeiterverwaltung zeigt die App „SSO: verknüpft“. Mit **„Verknüpfung lösen“** wird die Zuordnung entfernt; beim nächsten SSO-Login wird neu über den Benutzernamen verknüpft. Verknüpfen und Lösen stehen im Audit-Log.
5. **Abmelden** meldet auch beim Provider ab, sofern dieser einen `end_session_endpoint` anbietet (Authentik ja; Authelia je nach Version nicht, dann endet nur die App-Sitzung).

Jedes App-Token wird bei jeder Anfrage gegen den aktuellen Benutzerstand geprüft: Ein deaktiviertes Konto ist sofort abgemeldet, eine geänderte Rolle gilt sofort.

## Konfiguration der App

In `config.yaml`:

```yaml
auth:
  oidc:
    enabled: true
    issuer: "https://auth.example.de/application/o/zeiterfassung/"   # siehe Provider unten
    client_id: "nfc-zeiterfassung"
    client_secret: "…"
    redirect_url: "https://zeit.example.de/api/v1/auth/oidc/callback"
    # Optional:
    # scopes: [openid, profile, email]          # Standard
    # username_claim: preferred_username        # Standard
    # auto_link: true                           # Standard; false = Verknüpfen nur für bereits verknüpfte Konten
    # password_login: all                       # all | admins | none
    # button_label: "Mit Kita-Konto anmelden"
```

Oder per Umgebungsvariablen (z. B. in `docker-compose.yml`): `NFC_OIDC_ENABLED`, `NFC_OIDC_ISSUER`, `NFC_OIDC_CLIENT_ID`, `NFC_OIDC_CLIENT_SECRET`, `NFC_OIDC_REDIRECT_URL`, `NFC_OIDC_SCOPES`, `NFC_OIDC_USERNAME_CLAIM`, `NFC_OIDC_AUTO_LINK`, `NFC_OIDC_PASSWORD_LOGIN`, `NFC_OIDC_BUTTON_LABEL`.

**`password_login`** steuert die bisherige Anmeldung mit Benutzername und Passwort:

| Wert | Wirkung |
| --- | --- |
| `all` (Standard) | Alle können sich weiter auch mit Passwort anmelden. Gut für die Umstellungsphase. |
| `admins` | Nur Leitung und Superadmin dürfen mit Passwort anmelden (Fallback, falls der Provider ausfällt). Auf der Login-Seite ist das Passwortfeld eingeklappt. **Empfohlen**, sobald alle Mitarbeitenden verknüpft sind. |
| `none` | Nur noch SSO. Fällt der Provider aus, kommt niemand mehr in die App. |

`redirect_url` muss die Adresse sein, unter der Browser die App von außen erreichen. Beginnt sie mit `https://`, wird das Cookie für den Anmeldevorgang als `Secure` gesetzt. Die App startet auch, wenn der Provider gerade nicht erreichbar ist; die Verbindung wird erst beim ersten SSO-Login aufgebaut.

## Authentik

Ausführliche Schritt-für-Schritt-Anleitung mit Fehlersuche: [`sso-authentik.md`](sso-authentik.md). Kurzfassung:

1. **Applications → Providers → Create → OAuth2/OpenID Provider**
   - Client type: **Confidential**
   - Redirect URIs: `https://zeit.example.de/api/v1/auth/oidc/callback` (strict)
   - **Signing Key: ein Zertifikat auswählen** (z. B. „authentik Self-signed Certificate“). Ohne Signing Key signiert Authentik mit HS256, das die App nicht annimmt.
   - Scopes: `openid`, `profile`, `email` (Standard)
2. **Applications → Create**, Slug z. B. `zeiterfassung`, Provider auswählen. Über Bindings festlegen, welche Gruppe die App nutzen darf.
3. In der App eintragen:
   - `issuer`: `https://authentik.example.de/application/o/zeiterfassung/` (mit Schrägstrich am Ende, wie in „OpenID Configuration Issuer“ angezeigt)
   - `client_id` / `client_secret`: aus dem Provider

Der Authentik-Benutzername landet in `preferred_username`.

## Authelia

In der Authelia-Konfiguration (ab 4.38):

```yaml
identity_providers:
  oidc:
    # hmac_secret und jwks wie in der Authelia-Doku beschrieben
    clients:
      - client_id: 'nfc-zeiterfassung'
        client_name: 'NFC Zeiterfassung'
        # Hash erzeugen: authelia crypto hash generate pbkdf2 --random
        client_secret: '$pbkdf2-sha512$310000$…'
        public: false
        authorization_policy: 'two_factor'
        require_pkce: true
        pkce_challenge_method: 'S256'
        redirect_uris:
          - 'https://zeit.example.de/api/v1/auth/oidc/callback'
        scopes: ['openid', 'profile', 'email']
        token_endpoint_auth_method: 'client_secret_basic'
```

In der App: `issuer: "https://auth.example.de"`, `client_id` und das **Klartext**-Secret (nicht den Hash). Neuere Authelia-Versionen liefern den Benutzernamen nur über den Userinfo-Endpunkt; die App fragt ihn dort automatisch ab.

## Reverse Proxy

Für SSO braucht es **kein** Forward-Auth vor der App: Die App fragt den Provider selbst. Der Proxy (Caddy, Traefik, Nginx, Cloudflare Tunnel …) leitet einfach auf den Container weiter.

- Von außen nur die Weboberfläche und `/api/v1/...` freigeben. Die Android-Stempelgeräte sprechen mit dem Server im LAN (`/api/v1/device/...`); diese Pfade müssen nicht von außen erreichbar sein.
- Wer zusätzlich Forward-Auth (z. B. Authelia oder den Authentik-Outpost) vor die ganze Seite setzt, sollte `/api/v1/device/` davon ausnehmen. Mit bestehender SSO-Sitzung merkt man das zweite Tor nicht.
- Der Proxy sollte `X-Forwarded-For` setzen, damit das Login-Rate-Limit pro Client-IP greift.
