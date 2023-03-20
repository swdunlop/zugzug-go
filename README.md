# Zug Zug Go!

Zug Zug is a package that organizes tasks written in Go so they can be run serially or in parallel.  It is heavily
influences by [Mage](https://magefile.org/), but foregoes code generation in favor of being easier to embed and extend.
(There would be no Zug without the lessons learned from using Mage over the years!)

## Differences from Mage

- Zug does not have a CLI like Mage that uses Go to parse your targets and generate a binary. Instead, Zug just has 
  simple packages with options for organizing and running your tasks.

- Zug uses Go contexts to alter how tasks are run, such as redirecting output to a Go buffer or even bending the rules
  about re-running tasks.  This is a powerful feature that you can easily ignore until you need it.

- Zug is more finicky (and arguably more Go-like) about errors and interruption.  Zug will not stop concurrent tasks
  when one fails, instead it will wait for all of them to finish and return all of the errors.  This makes Zug a more
  of a pest for writing quick and dirty tasks, but a much better citizen as a package that can be embedded in other
  projects.

- Tasks can be more than just Go functions, if they take on the responsibility of ensuring they only run once (or are
  idempoent).  You can still just use simple Go functions and Zug Zug will handle the bookkeeping in the background.
  (In fact, Zug can cheat around the "only once" rule so you can run a task repeatedly -- see the "worker" package.)

## Getting Started

Zug Zug is a Go module, so you can just import it into your project and start using its packages.  See [./examples](./examples) for some simple examples of how to use Zug Zug.

## License

Zug Zug is licensed under the BSD 3-Clause License.  See [LICENSE](./LICENSE) for details.

## Contributing

Zug Zug is very much in early development, the pieces work, but there is more to add in terms of options, especially in
the "zugzug" and "console" packages.  If you have ideas or want to contribute, please open an issue or pull request.
