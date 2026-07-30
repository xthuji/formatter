// Go edge cases: imports sorting, struct literals
package main

import (
"fmt"
"os"
)

type Point struct{X int;Y int}
func main(){
p:=Point{X:1,Y:2}
fmt.Printf("Point: %+v\n",p)
if p.X>0{os.Exit(0)}
}
