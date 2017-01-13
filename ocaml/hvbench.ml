(* A Hyper-V socket benchmarking program
 *
 * This is a direct replacement for c/hvbench
 *)

type mode =
  | Server
  | Client of Hvsock.vmid
type test =
  | Bandwidth
  | Latency
  | Connection

let bm_guid = "3049197C-9A4E-4FBF-9367-97F792F16994"

let verbose = ref 0

let info  fmt = Printf.ksprintf (fun s -> if !verbose > 0 then print_string s) fmt
let debug fmt = Printf.ksprintf (fun s -> if !verbose > 1 then print_string s) fmt
let trc   fmt = Printf.ksprintf (fun s -> if !verbose > 2 then print_string s) fmt

(* There's anecdotal evidence that a blocking send()/recv() is slower
 * than performing non-blocking send()/recv() calls and then use
 * epoll()/WSAPoll().  This flags switches between the two
 *)
let opt_poll = ref false

(* There's anecdotal evidence that the Lwt layer in hvsock is a problem.
   Requesting blocking bypasses this. *)
let blocking = ref false

(* Use a static buffer for send and receive. *)
let buf = Cstruct.create (2 * 1024 * 1024)

(* Time (in ns) to run eeach bandwidth test *)
let bm_bw_time = 10_000_000_000L

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
  Lwt.catch
    (fun () ->
      Lwt_hvsock.read fd (Cstruct.sub buf 0 msg_sz)
      >>= function
      | 0 ->
        trc "Received: 0\n";
        Lwt.return_unit
      | n ->
        trc "Received: %d\n" n;
        bw_rx fd msg_sz
    ) (function
      | Unix.Unix_error(Unix.ECONNRESET, _, _) ->
        trc "Received ECONNRESET\n";
        Lwt.return_unit
      | e -> Lwt.fail e
    )

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
    info "server: listening\n";
    let max_conn = match opt_bm with Connection -> bm_conns | _ -> 1 in
    let rec loop = function
      | 0 ->
        Lwt.return_unit
      | n ->
        Lwt_hvsock.accept lsock
        >>= fun (csock, _) ->
        info "server: accepted\n";
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
  failwith "Client connections benchmark unimplemented"

let bw_tx_blocking fd msg_sz =
  let to_send = String.make msg_sz '\000' in
  let c = Mtime.counter () in
  let rec loop msgs_sent =
    if Mtime.(to_ns_uint64 @@ count c) > bm_bw_time
    then msgs_sent
    else begin
      let rec aux ofs len =
        if len > 0 then begin
          let n = Unix.write fd to_send ofs len in
          if n = 0 then failwith "write returned 0";
          aux (ofs + n) (len - n)
        end in
      aux 0 (String.length to_send);
      loop (Int64.add msgs_sent 1L)
    end in
  let msgs_sent = loop 0L in
  let ns = Mtime.(to_ns_uint64 @@ count c) in
  debug "bw_tx: %Ld %Ld %Ld\n" msgs_sent 0L ns;
  let ( ** ) = Int64.mul and ( // ) = Int64.div in
  (* bandwidth in Mbits per second *)
  let ms = ns // 1_000_000L in
  let bw = 8L ** msgs_sent ** (Int64.of_int msg_sz) ** 1000L // (ms ** 1024L ** 1024L) in
  bw

let bw_tx fd msg_sz =
  let open Lwt.Infix in
  let to_send = Cstruct.sub buf 0 msg_sz in
  let c = Mtime.counter () in
  let rec loop msgs_sent =
    if Mtime.(to_ns_uint64 @@ count c) > bm_bw_time
    then Lwt.return msgs_sent
    else begin
      Lwt_cstruct.complete (Lwt_hvsock.write fd) to_send
      >>= fun () ->
      loop (Int64.add msgs_sent 1L)
    end in
  loop 0L
  >>= fun msgs_sent ->
  let ns = Mtime.(to_ns_uint64 @@ count c) in
  debug "bw_tx: %Ld %Ld %Ld\n" msgs_sent 0L ns;
  let ( ** ) = Int64.mul and ( // ) = Int64.div in
  (* bandwidth in Mbits per second *)
  let ms = ns // 1_000_000L in
  let bw = 8L ** msgs_sent ** (Int64.of_int msg_sz) ** 1000L // (ms ** 1024L ** 1024L) in
  Lwt.return bw

let client target _bm msg_sz =
  let open Lwt.Infix in
  info "client: msg_sz=%d\n" msg_sz;
  let sa = {
    Hvsock.vmid = target;
    serviceid = bm_guid;
  } in
  if !blocking then begin
    let fd = Hvsock.create () in
    Hvsock.connect fd sa;
    let bw = bw_tx_blocking fd msg_sz in
    Printf.printf "%d %Ld\n" msg_sz bw;
    Unix.close fd
  end else Lwt_main.run begin
    let fd = Lwt_hvsock.create () in
    Lwt_hvsock.connect fd sa
    >>= fun () ->
    info "client: connected\n";
    bw_tx fd msg_sz
    >>= fun bw ->
    Printf.printf "%d %Ld\n" msg_sz bw;
    Lwt_hvsock.close fd
  end

let _ =
  let mode = ref Server in
  let test = ref Bandwidth in
  let msize = ref (Cstruct.len buf) in
  Arg.parse [
    "-s", Arg.Unit (fun () -> mode := Server), "Server mode";
    "-c", Arg.String (
        function
        | "loopback" -> mode := Client Hvsock.Loopback
        | "parent"   -> mode := Client Hvsock.Parent
        | guid       -> mode := Client (Hvsock.Id guid)
      ), "Client mode: 'loopback', 'parent' or '<guid>'";
    "-B", Arg.Unit (fun () -> test := Bandwidth), "Bandwidth test";
    "-L", Arg.Unit (fun () -> test := Latency), "Latency test";
    "-C", Arg.Unit (fun () -> test := Connection), "Connection test";
    "-m", Arg.Set_int msize, "Message size in bytes";
    "-p", Arg.Set opt_poll, "Use poll instead of blocking send()/recv()";
    "-v", Arg.Unit (fun () -> incr verbose), "Verbose output";
    "-blocking", Arg.Set blocking, "Use blocking I/O";
  ] (fun unexpected_arg ->
    Printf.fprintf stderr "Unexpected argument: %s\nSee -help for usage\n" unexpected_arg;
    exit 1
  ) "hvbench: a Hyper-V socket benchmarking program";

  match !mode, !test with
  | Server, opt_bm -> server opt_bm (!msize)
  | Client target, Connection -> client_conn target
  | Client target, opt_bm -> client target opt_bm (!msize)
