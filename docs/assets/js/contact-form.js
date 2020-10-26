
/* Contact form */
$(function() {
  var submitButton = $("#submit-button");
  var form = $("#contact-form");
  
  selectPlanFromUrl();
  
  submitButton.on("click", submitForm);
  
  function selectPlanFromUrl() {
    var params = window.location.search;
    if (params === "?plan=enterprise") {
      var plan = $("#enterprise");
      plan.prop('checked', true);
    } else {
      var plan = $("#pro");
      plan.prop('checked', true);
    }
  }

  function submitForm(e) {
    e.preventDefault();

    if(validateForm()) {
      var data = form.serialize(form.get(0), { hash: true });
      submitToFormSpree(data);
    }

    function submitToFormSpree(data) {
      submitButton.html("Sending...");
      submitButton.prop("disabled", true);
      var postParams = {
        url: form.attr('action'),
        type: "POST",
        data: data,
        dataType: "json"
      };

      $.ajax(postParams)
        .done(function() {
          inCall = false;
          window.location.replace("/thanks");
        })
        .fail(function(error) {
          showFormError(
            "Oops, something went wrong! Please try again. If the issue persists please email us directly at info@gruntwork.io"
          );
          inCall = false;
          submitButton.html("Submit");
          submitButton.prop("disabled", false);
        });
    }
    
    function showInputError(el) {
      $(el).addClass("has-error");
    };
    
    function showFormError(message) {
      $("#error-message").html(
        '<h3 class="text-danger text-center">' + message + "</h3>"
        );
      };
      
      function clearErrors() {
        $("#error-message").html("");
        form.find("*").removeClass("has-error");
      };

     function validateForm() {
        var isValid = true;
    
        clearErrors();
    
        form.find("[required]").each(function(index, el) {
          if (!$(el).val()) {
            isValid = false;
            showInputError(el);
            showFormError("Please fill in all required fields");
          }
        });
    
        return isValid;
      };
    }
  });