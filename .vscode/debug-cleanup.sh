#!/bin/sh
if [ "$1" = "yes" ]; then
    make run-reset
else
    echo "Skipping cleanup (resources kept)."
fi
