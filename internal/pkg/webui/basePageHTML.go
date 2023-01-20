package webui

const basePageHTML = `<!DOCTYPE html PUBLIC " - //W3C//DTD xhtml 1.0 Strict//EN"	"http://www.w3.org/1999/xhtml">
<head>
    <style>
     <!-- Auto-fit generated graphs to the browser window -->
     <style>
     html, body {
	 height: 100%;
     }
     img, object {
	 display: block;
	 max-height: 100%;
	 max-width: 100%;
     }
    </style>
    <style>
    <!-- Enable pop-up windows -->
     /* Popup container - can be anything you want */
     .popup {
	 position: relative;
	 display: inline-block;
	 cursor: pointer;
	 -webkit-user-select: none;
	 -moz-user-select: none;
	 -ms-user-select: none;
	 user-select: none;
     }

     /* The actual popup */
     .popup .popuptext {
	 visibility: hidden;
	 width: 160px;
	 background-color: #555;
	 color: #fff;
	 text-align: center;
	 border-radius: 6px;
	 padding: 8px 0;
	 position: absolute;
	 z-index: 1;
	 bottom: 125%;
	 left: 50%;
	 margin-left: -80px;
     }

     /* Popup arrow */
     .popup .popuptext::after {
	 content: "";
	 position: absolute;
	 top: 100%;
	 left: 50%;
	 margin-left: -5px;
	 border-width: 5px;
	 border-style: solid;
	 border-color: #555 transparent transparent transparent;
     }

     /* Toggle this class - hide and show the popup */
     .popup .show {
	 visibility: visible;
	 -webkit-animation: fadeIn 1s;
	 animation: fadeIn 1s;
     }

     /* Add animation (fade in the popup) */
     @-webkit-keyframes fadeIn {
	 from {opacity: 0;}
	 to {opacity: 1;}
     }

     @keyframes fadeIn {
	 from {opacity: 0;}
	 to {opacity:1 ;}
     }
    </style>
    {{block "head" . -}}
	<title>Korrel8r Web UI</title>
    {{end -}}
</head>
<body>
    {{block "body" . -}}
	Nothing to see here.
    {{end -}}
</body>
`
