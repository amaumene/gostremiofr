package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleConfig serves the configuration HTML page
func (h *Handler) handleConfig(c *gin.Context) {
	html := `<!DOCTYPE html>
<html lang="fr">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Configuration GoStremioFR</title>
  <link href="https://fonts.googleapis.com/css2?family=Roboto:wght@400;500;700&display=swap" rel="stylesheet">
  <style>
    :root {
      --primary-color: #4a90e2;
      --secondary-color: #50e3c2;
      --background-color: #f7f9fc;
      --text-color: #333;
      --input-border: #ccc;
      --input-focus: var(--primary-color);
    }
    * { box-sizing: border-box; }
    body {
      font-family: 'Roboto', sans-serif;
      background-color: var(--background-color);
      color: var(--text-color);
      margin: 0;
      padding: 20px;
      display: flex;
      align-items: center;
      justify-content: center;
      min-height: 100vh;
    }
    .container {
      background-color: #fff;
      border-radius: 8px;
      padding: 30px;
      max-width: 500px;
      width: 100%;
      box-shadow: 0 4px 12px rgba(0, 0, 0, 0.1);
    }
    h1 {
      text-align: center;
      margin-bottom: 20px;
      color: var(--primary-color);
    }
    label { font-weight: 500; margin-top: 15px; display: block; }
    input {
      width: 100%;
      padding: 10px;
      border: 1px solid var(--input-border);
      border-radius: 4px;
      margin-top: 5px;
      font-size: 1rem;
    }
    input:focus {
      outline: none;
      border-color: var(--input-focus);
      box-shadow: 0 0 5px rgba(74, 144, 226, 0.5);
    }
    button {
      background-color: var(--primary-color);
      color: #fff;
      border: none;
      padding: 12px 20px;
      border-radius: 4px;
      font-size: 1rem;
      cursor: pointer;
      margin-top: 25px;
      width: 100%;
      transition: background-color 0.3s ease;
    }
    button:hover { background-color: var(--secondary-color); }
    .result {
      margin-top: 25px;
      background-color: #f1f3f5;
      border: 1px solid #e0e6ed;
      border-radius: 4px;
      padding: 15px;
      word-break: break-all;
    }
    .result a {
      color: var(--primary-color);
      text-decoration: none;
      font-weight: 500;
    }
    .result a:hover { text-decoration: underline; }
  </style>
  <script>
    function getConfigFromURL() {
      const pathParts = window.location.pathname.split('/').filter(p => p);
      if (pathParts.length >= 2 && pathParts[pathParts.length - 1] === "configure") {
        try {
          const encodedConfig = pathParts[pathParts.length - 2];
          const decodedConfig = JSON.parse(atob(encodedConfig));

          document.getElementById('tmdb').value = decodedConfig.TMDB_API_KEY || "";
          document.getElementById('res').value = (decodedConfig.RES_TO_SHOW || []).join(",");
          document.getElementById('lang').value = (decodedConfig.LANG_TO_SHOW || []).join(",");
          document.getElementById('alldebrid').value = decodedConfig.API_KEY_ALLDEBRID || "";
          
        } catch (error) {
          console.error("Error decoding configuration:", error);
        }
      }
    }

    function generateConfig() {
      const config = {
        TMDB_API_KEY: document.getElementById('tmdb').value,
        RES_TO_SHOW: document.getElementById('res').value.split(',').map(s => s.trim()).filter(s => s),
        LANG_TO_SHOW: document.getElementById('lang').value.split(',').map(s => s.trim()).filter(s => s),
        API_KEY_ALLDEBRID: document.getElementById('alldebrid').value
      };
      const encodedConfig = btoa(JSON.stringify(config));
      
      const protocol = window.location.protocol;
      const host = window.location.hostname;
      const port = window.location.port ? ':' + window.location.port : '';
      const baseUrl = protocol + '//' + host + port;
      
      document.getElementById('result').innerHTML = 
        '<p><strong>Configuration encodée (base64):</strong></p>' +
        '<p>' + encodedConfig + '</p>' +
        '<p><strong>Lien de configuration:</strong></p>' +
        '<p>' +
          '<a href="' + baseUrl + '/' + encodedConfig + '/configure" target="_blank">' +
            baseUrl + '/' + encodedConfig + '/configure' +
          '</a>' +
        '</p>' +
        '<p><strong>Lien du manifest:</strong></p>' +
        '<p>' +
          '<a href="' + baseUrl + '/' + encodedConfig + '/manifest.json" target="_blank">' +
            baseUrl + '/' + encodedConfig + '/manifest.json' +
          '</a>' +
        '</p>' +
        '<p><strong>Exemple de lien stream (film):</strong></p>' +
        '<p>' +
          '<a href="' + baseUrl + '/' + encodedConfig + '/stream/movie/tt1234567.json" target="_blank">' +
            baseUrl + '/' + encodedConfig + '/stream/movie/tt1234567.json' +
          '</a>' +
        '</p>';
    }

    window.onload = getConfigFromURL;
  </script>
</head>
<body>
  <div class="container">
    <h1>Configuration GoStremioFR</h1>
    <label for="tmdb">Clé API TMDB</label>
    <input type="text" id="tmdb" placeholder="Entrez votre clé API TMDB">
    
    <label for="res">Résolutions (séparées par une virgule)</label>
    <input type="text" id="res" value="2160p,1080p,720p,480p" placeholder="Ex: 2160p,1080p,720p">
    
    <label for="lang">Langues (séparées par une virgule)</label>
    <input type="text" id="lang" value="MULTI,FRENCH,VOSTFR" placeholder="Ex: MULTI,FRENCH,VOSTFR">
    
    <label for="alldebrid">Clé API AllDebrid</label>
    <input type="text" id="alldebrid" placeholder="Entrez votre clé API AllDebrid">
    
    
    <button onclick="generateConfig()">Générer la configuration</button>
    <div id="result" class="result"></div>
  </div>
</body>
</html>`

	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// handleConfigWithParams serves the config page with pre-filled values from URL
func (h *Handler) handleConfigWithParams(c *gin.Context) {
	// The JavaScript in the HTML will handle parsing the configuration from the URL
	h.handleConfig(c)
}
