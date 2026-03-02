(function() {
  var dropZone = document.getElementById("drop-zone");
  var fileInput = document.getElementById("file-input");
  var fileList = document.getElementById("file-list");
  var uploadBtn = document.getElementById("upload-btn");
  var pendingFiles = [];

  dropZone.addEventListener("dragover", function(e) {
    e.preventDefault();
    dropZone.classList.add("drag-over");
  });

  dropZone.addEventListener("dragleave", function(e) {
    e.preventDefault();
    dropZone.classList.remove("drag-over");
  });

  dropZone.addEventListener("drop", function(e) {
    e.preventDefault();
    dropZone.classList.remove("drag-over");
    addFiles(e.dataTransfer.files);
  });

  dropZone.addEventListener("click", function() {
    fileInput.click();
  });

  fileInput.addEventListener("change", function() {
    addFiles(fileInput.files);
    fileInput.value = "";
  });

  function addFiles(fileListObj) {
    for (var i = 0; i < fileListObj.length; i++) {
      pendingFiles.push(fileListObj[i]);
    }
    renderList();
  }

  function renderList() {
    fileList.innerHTML = "";
    for (var i = 0; i < pendingFiles.length; i++) {
      var row = document.createElement("tr");

      var nameCell = document.createElement("td");
      nameCell.textContent = pendingFiles[i].name;
      row.appendChild(nameCell);

      var sizeCell = document.createElement("td");
      sizeCell.textContent = formatSize(pendingFiles[i].size);
      row.appendChild(sizeCell);

      var statusCell = document.createElement("td");
      statusCell.setAttribute("id", "status-" + i);
      statusCell.textContent = "Queued";
      row.appendChild(statusCell);

      var progressCell = document.createElement("td");
      var progress = document.createElement("progress");
      progress.setAttribute("id", "progress-" + i);
      progress.setAttribute("max", "100");
      progress.setAttribute("value", "0");
      progress.style.width = "8em";
      progressCell.appendChild(progress);
      row.appendChild(progressCell);

      var removeCell = document.createElement("td");
      var removeBtn = document.createElement("input");
      removeBtn.type = "button";
      removeBtn.value = "Remove";
      removeBtn.setAttribute("data-index", i);
      removeBtn.addEventListener("click", function() {
        var idx = parseInt(this.getAttribute("data-index"));
        pendingFiles.splice(idx, 1);
        renderList();
      });
      removeCell.appendChild(removeBtn);
      row.appendChild(removeCell);

      fileList.appendChild(row);
    }
    uploadBtn.style.display = pendingFiles.length > 0 ? "" : "none";
  }

  uploadBtn.addEventListener("click", function() {
    if (pendingFiles.length === 0) return;
    uploadBtn.disabled = true;
    // Disable all remove buttons
    var removeBtns = fileList.querySelectorAll("input[type=button]");
    for (var i = 0; i < removeBtns.length; i++) {
      removeBtns[i].disabled = true;
    }
    uploadNext(0);
  });

  function uploadNext(index) {
    if (index >= pendingFiles.length) {
      // All done — reload page to show updated file list
      window.location.reload();
      return;
    }

    var statusCell = document.getElementById("status-" + index);
    var progressBar = document.getElementById("progress-" + index);
    statusCell.textContent = "Uploading…";

    var formData = new FormData();
    formData.append("file", pendingFiles[index]);

    var xhr = new XMLHttpRequest();

    xhr.upload.addEventListener("progress", function(e) {
      if (e.lengthComputable) {
        var pct = Math.round((e.loaded / e.total) * 100);
        progressBar.value = pct;
        statusCell.textContent = "Uploading… " + pct + "%";
      }
    });

    xhr.addEventListener("load", function() {
      if (xhr.status >= 200 && xhr.status < 400) {
        progressBar.value = 100;
        statusCell.textContent = "Done";
      } else {
        statusCell.textContent = "Error";
      }
      uploadNext(index + 1);
    });

    xhr.addEventListener("error", function() {
      statusCell.textContent = "Error";
      uploadNext(index + 1);
    });

    xhr.open("POST", "/files/upload");
    xhr.send(formData);
  }

  function formatSize(bytes) {
    if (bytes < 1024) return bytes + " B";
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + " KB";
    return (bytes / (1024 * 1024)).toFixed(1) + " MB";
  }

  renderList();
})();
