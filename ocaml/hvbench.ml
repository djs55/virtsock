(* A Hyper-V socket benchmarking program
 *
 * This is a direct replacement for c/hvbench
 *)

type address =
  | Loopback
  | Parent
  | Guid of string
type mode =
  | Server
  | Client of address
type test =
  | Bandwidth
  | Latency
  | Connection

let mode = ref Server
let test = ref Bandwidth
let msize = ref 1024
let use_poll = ref false
let verbose = ref false

let _ =
  Arg.parse [
    "-s", Arg.Unit (fun () -> mode := Server), "Server mode";
    "-c", Arg.String (
        function
        | "loopback" -> mode := Client Loopback
        | "parent"   -> mode := Client Parent
        | guid       -> mode := Client (Guid guid)
      ), "Client mode: 'loopback', 'parent' or '<guid>'";
    "-B", Arg.Unit (fun () -> test := Bandwidth), "Bandwidth test";
    "-L", Arg.Unit (fun () -> test := Latency), "Latency test";
    "-C", Arg.Unit (fun () -> test := Connection), "Connection test";
    "-m", Arg.Set_int msize, "Message size in bytes";
    "-p", Arg.Set use_poll, "Use poll instead of blocking send()/recv()";
    "-v", Arg.Set verbose, "Verbose output"
  ] (fun unexpected_arg ->
    Printf.fprintf stderr "Unexpected argument: %s\nSee -help for usage\n" unexpected_arg;
    exit 1
  ) "hvbench: a Hyper-V socket benchmarking program";
