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

let verbose = ref 0

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

(* We'll use normal Lwt_unix rather than Uwt *)
module Time = struct
  type 'a io = 'a Lwt.t
  let sleep = Lwt_unix.sleep
end
module Main = struct
  let run_in_main = Lwt_preemptive.run_in_main
  let run = Lwt_main.run
end
module Lwt_hvsock = Lwt_hvsock.Make(Time)(Main)

(* Bandwidth tests:
 *
 * The TX side sends a fixed amount of data in fixed sized
 * messages. The RX side drains the ring in message sized chunks (or less).
 *)
let rec bw_rx fd msg_sz =
  let open Lwt.Infix in
  Lwt_hvsock.read fd buf
  >>= function
  | 0 ->
    Lwt.return_unit
  | n ->
    if !verbose > 0 then Printf.printf "Received: %d\n" n;
    bw_rx fd msg_sz

(* Server:
 * accept() in an endless loop, handle a connection at a time
 *)
let server opt_bm msg_sz =
  let sa = {
    Hvsock.vmid = Hvsock.Wildcard;
    serviceid = bm_guid;
  } in
  Lwt_main.run begin
    let open Lwt.Infix in
    let lsock = Lwt_hvsock.create () in
    Lwt_hvsock.bind lsock sa;
    Lwt_hvsock.listen lsock 5;
    Printf.printf "server: listening\n";
    let max_conn = match opt_bm with Connection -> bm_conns | _ -> 1 in
    let rec loop = function
      | 0 ->
        Lwt.return_unit
      | n ->
        Lwt_hvsock.accept lsock
        >>= fun (csock, _) ->
        Printf.printf "server: accepted\n";
        bw_rx csock msg_sz
        >>= fun () ->
        Lwt_hvsock.close csock
        >>= fun () ->
        loop (n - 1) in
    loop max_conn
    >>= fun () ->
    Lwt_hvsock.close lsock
  end

let client_conn _target =
  failwith "Client unimplemented"
let client _target _opt_bm _opt_msgsz =
  failwith "Client unimplemented"

let _ =
  let mode = ref Server in
  let test = ref Bandwidth in
  let msize = ref (Cstruct.len buf) in
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
    "-v", Arg.Unit (fun () -> incr verbose), "Verbose output"
  ] (fun unexpected_arg ->
    Printf.fprintf stderr "Unexpected argument: %s\nSee -help for usage\n" unexpected_arg;
    exit 1
  ) "hvbench: a Hyper-V socket benchmarking program";

  match !mode, !test with
  | Server, opt_bm -> server opt_bm (!msize)
  | Client target, Connection -> client_conn target
  | Client target, opt_bm -> client target opt_bm (!msize)
