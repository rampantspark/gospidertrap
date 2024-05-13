This program is inspired by [spidertrap](https://github.com/adhdproject/spidertrap).

HTML Input (link replacement)
 - Provide an HTML file as input and the links with be replaced with randomly generated ones.

HTML Input (form submit action)
  - Provide an HTML file containing a form that submits a get request to endpoint and links will be procedurally generated on form submit.

Wordlist
  - All links can be picked from a wordlist instead of procedurally generated. 

Usage:

```
go run main.go
  -p - Port
  -a - HTML file input, replace <a href> links
  -e - HTML file input, form get requests point at endpoint that returns link
  -w - Wordlist to use for links

```
