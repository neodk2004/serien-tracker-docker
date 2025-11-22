[![Test Docker Build and Run](https://github.com/neodk2004/serien-tracker-docker/actions/workflows/serien-tracker-docker.yml/badge.svg?branch=main)](https://github.com/neodk2004/serien-tracker-docker/actions/workflows/serien-tracker-docker.yml)
# Serientracker (Go)
Ein einfacher und effizienter Serientracker, geschrieben in Go, der die OMDb API nutzt, um Serieninformationen abzurufen und persönliche Serienlisten zu verwalten.
<img width="1771" height="761" alt="Screenshot 2025-11-22 124254" src="https://github.com/user-attachments/assets/c1de464b-49ac-4e0f-abe4-801a56373de1" />

💡 Features 

🔐 Login für 4 Nutzer (A, B, C, D)
📁 Getrennte Serienlisten pro Nutzer
👮 Admin-Panel (nur für Nutzer A)
🌐 IMDb-Integration (Suche & Cover)
📄 PDF-Export deiner Liste
🐳 Vollständig in Docker containerisiert

# Funktionen
Serien hinzufügen über Titel oder IMDb-ID
<img width="747" height="185" alt="Screenshot 2025-11-22 124606" src="https://github.com/user-attachments/assets/d8042626-d9bd-4900-92a8-7a9c184d5bee" />

Folgenstatus verwalten (Anzahl der gesehenen Folgen)</br>
<img width="288" height="282" alt="Screenshot 2025-11-22 124648" src="https://github.com/user-attachments/assets/6f1ef9f6-343d-42be-929c-90625517a7cd" />


Vollständige Serieninformationen (Titel, Staffeln, Episoden, Bewertung, etc.)

PDF-Export der Serienliste zum Teilen mit Freunden
<img width="1414" height="253" alt="Screenshot 2025-11-22 124724" src="https://github.com/user-attachments/assets/dd78555a-d19e-461e-b5b2-bbf2b0e468b1" />

Lokale Datenspeicherung im JSON-Format

# 🛠️ Voraussetzungen
Docker (v20.10 oder höher)
Docker Compose (in neueren Docker-Versionen bereits enthalten)
Ein kostenloser OMDb API-Key

# 🚀 Schnellstart (Lokal)
Du brauchst kein Go installiert – alles läuft in Docker!

1. Repository klonen

          git clone https://github.com/neodk2004/serien-tracker.git
          cd serien-tracker

🔽 Warum klonen?
Deine Anwendung wird direkt aus dem Quellcode gebaut – daher benötigt Docker Zugriff auf Dockerfile, main.go, templates/ etc. 

2. API-Key hinzufügen
Erstelle eine Datei .env im Projektordner:

        cp .env.example .env
Öffne .env und trage deinen echten OMDb-API-Key ein:

        env
        OMDb_API_KEY=dein_echter_api_key_hier

📌 Du brauchst einen kostenlosen Key von https://www.omdbapi.com/apikey.aspx 

3. Mit einem Befehl starten

        docker-compose up --build
Docker baut automatisch das Image
Startet den Container
Macht die App auf http://localhost:8080 verfügbar

💡 Kein manuelles docker build nötig – docker-compose erledigt alles! 

4. Loslegen!

Öffne http://localhost:8080
Wähle einen Nutzer (z. B. Nutzer A für Admin-Zugriff)
Füge deine ersten Serien hinzu!

🔁 Ohne erneutes Bauen starten (bei wiederholtem Start)
Nach dem ersten --build genügt:

    docker-compose up

Deine Daten bleiben erhalten – sie werden im lokalen Ordner ./data/ gespeichert.

🗑️ Aufräumen (optional)
Stoppe und entferne Container:

    docker-compose down

Willst du alle Nutzerdaten löschen?

    rm -rf data/

✅ Das ist alles! Kein Go, kein Build-Tool – nur Docker und ein API-Key.







