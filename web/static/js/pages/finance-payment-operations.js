document.addEventListener('DOMContentLoaded', function () {
    'use strict';

    // Keep filter submission keyboard-friendly without putting behavior in the
    // template. Destructive recovery forms use the shared confirmation hook.
    var filter = document.querySelector('[data-operations-filter]');
    if (filter) {
        var query = filter.querySelector('input[name="q"]');
        if (query) {
            query.addEventListener('keydown', function (event) {
                if (event.key === 'Enter') {
                    event.currentTarget.form.submit();
                }
            });
        }
    }
});
