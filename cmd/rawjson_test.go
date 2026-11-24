package cmd

// Text with "&", "<" and ">", which must reach --json as the server sent it,
// not as \u0026, \u003c and \u003e.
const htmlText = `Simon & Garfunkel <3 > the rest`
