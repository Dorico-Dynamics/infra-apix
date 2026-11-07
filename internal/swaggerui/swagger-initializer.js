// Starts Swagger UI on the page apix serves. The page names the document in
// the data-url attribute, so this script is static and the page runs no inline
// script.
"use strict";

window.addEventListener("load", function () {
  var root = document.getElementById("swagger-ui");
  window.ui = SwaggerUIBundle({
    url: root.dataset.url,
    domNode: root,
    presets: [SwaggerUIBundle.presets.apis],
    layout: "BaseLayout"
  });
});
