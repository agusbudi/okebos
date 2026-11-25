// Format input uang dengan titik pemisah ribuan (1500000 -> 1.500.000).
document.querySelectorAll('input.rupiah').forEach(function (el) {
  el.addEventListener('input', function () {
    var digits = el.value.replace(/\D/g, '').replace(/^0+(?=\d)/, '');
    el.value = digits.replace(/\B(?=(\d{3})+(?!\d))/g, '.');
  });
});
