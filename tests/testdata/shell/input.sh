#!/bin/bash
# Shell test script
NAME="world"
if [ "$NAME" = "world" ]; then
echo "Hello, $NAME!"
for i in 1 2 3; do
echo "Number: $i"
done
fi
greet() {
echo "Hi, $1!"
}
greet "Formtter"
