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

let bm_guid = "3049197C-9A4E-4FBF-9367-97F792F16994"

(* There's anecdotal evidence that a blocking send()/recv() is slower
 * than performing non-blocking send()/recv() calls and then use
 * epoll()/WSAPoll().  This flags switches between the two
 *)
let opt_poll = ref false

(* Use a static buffer for send and receive. *)
let buf = Cstruct.create (2 * 1024 * 1024)

(* Time (in ns) to run eeach bandwidth test *)
let bm_bw_time = 10 * 1000 * 1000 * 1000

(* How many connections to make *)
let bm_conns = 2000

let server _opt_bm _opt_msgsz =
  failwith "Server unimplemented"
let client_conn _target =
  failwith "Client unimplemented"
let client _target _opt_bm _opt_msgsz =
  failwith "Client unimplemented"

let _ =
  let mode = ref Server in
  let test = ref Bandwidth in
  let msize = ref 1024 in
  let verbose = ref false in
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
    "-p", Arg.Set opt_poll, "Use poll instead of blocking send()/recv()";
    "-v", Arg.Set verbose, "Verbose output"
  ] (fun unexpected_arg ->
    Printf.fprintf stderr "Unexpected argument: %s\nSee -help for usage\n" unexpected_arg;
    exit 1
  ) "hvbench: a Hyper-V socket benchmarking program";

  match !mode, !test with
  | Server, opt_bm -> server opt_bm (!msize)
  | Client target, Connection -> client_conn target
  | Client target, opt_bm -> client target opt_bm (!msize)
