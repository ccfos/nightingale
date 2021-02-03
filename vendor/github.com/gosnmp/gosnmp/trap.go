// Copyright 2012 The GoSNMP Authors. All rights reserved.  Use of this
// source code is governed by a BSD-style license that can be found in the
// LICENSE file.

package gosnmp

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//
// Sending Traps ie GoSNMP acting as an Agent
//

// SendTrap sends a SNMP Trap (v2c/v3 only)
//
// pdus[0] can a pdu of Type TimeTicks (with the desired uint32 epoch
// time).  Otherwise a TimeTicks pdu will be prepended, with time set to
// now. This mirrors the behaviour of the Net-SNMP command-line tools.
//
// SendTrap doesn't wait for a return packet from the NMS (Network
// Management Station).
//
// See also Listen() and examples for creating an NMS.
//
// NOTE: the trap code is currently unreliable when working with snmpv3 - pull requests welcome
func (x *GoSNMP) SendTrap(trap SnmpTrap) (result *SnmpPacket, err error) {
	var pdutype PDUType

	if len(trap.Variables) == 0 {
		return nil, fmt.Errorf("function SendTrap requires at least 1 PDU")
	}

	if trap.Variables[0].Type == TimeTicks {
		// check is uint32
		if _, ok := trap.Variables[0].Value.(uint32); !ok {
			return nil, fmt.Errorf("function SendTrap TimeTick must be uint32")
		}
	}

	switch x.Version {
	case Version2c, Version3:
		// Default to a v2 trap.
		pdutype = SNMPv2Trap

		// If it's an inform, do that instead.
		if trap.IsInform {
			pdutype = InformRequest
		}

		if trap.Variables[0].Type != TimeTicks {
			now := uint32(time.Now().Unix())
			timetickPDU := SnmpPDU{"1.3.6.1.2.1.1.3.0", TimeTicks, now}
			// prepend timetickPDU
			trap.Variables = append([]SnmpPDU{timetickPDU}, trap.Variables...)
		}

	case Version1:
		pdutype = Trap
		if len(trap.Enterprise) == 0 {
			return nil, fmt.Errorf("function SendTrap for SNMPV1 requires an Enterprise OID")
		}
		if len(trap.AgentAddress) == 0 {
			return nil, fmt.Errorf("function SendTrap for SNMPV1 requires an Agent Address")
		}

	default:
		err = fmt.Errorf("function SendTrap doesn't support %s", x.Version)
		return nil, err
	}

	packetOut := x.mkSnmpPacket(pdutype, trap.Variables, 0, 0)
	if x.Version == Version1 {
		packetOut.Enterprise = trap.Enterprise
		packetOut.AgentAddress = trap.AgentAddress
		packetOut.GenericTrap = trap.GenericTrap
		packetOut.SpecificTrap = trap.SpecificTrap
		packetOut.Timestamp = trap.Timestamp
	}

	// all sends wait for the return packet, except for SNMPv2Trap
	// -> wait is only for informs
	return x.send(packetOut, trap.IsInform)
}

//
// Receiving Traps ie GoSNMP acting as an NMS (Network Management
// Station).
//
// GoSNMP.unmarshal() currently only handles SNMPv2Trap
//

// A TrapListener defines parameters for running a SNMP Trap receiver.
// nil values will be replaced by default values.
type TrapListener struct {
	sync.Mutex

	// Params is a reference to the TrapListener's "parent" GoSNMP instance.
	Params *GoSNMP

	// OnNewTrap handles incoming Trap and Inform PDUs.
	OnNewTrap TrapHandlerFunc

	// These unexported fields are for letting test cases
	// know we are ready.
	conn  *net.UDPConn
	proto string

	finish    int32 // Atomic flag; set to 1 when closing connection
	done      chan bool
	listening chan bool
}

// TrapHandlerFunc is a callback function type which receives SNMP Trap and
// Inform packets when they are received.  If this callback is null, Trap and
// Inform PDUs will not be received (Inform responses will still be sent,
// however).  This callback should not modify the contents of the SnmpPacket
// nor the UDPAddr passed to it, and it should copy out any values it wishes to
// use instead of retaining references in order to avoid memory fragmentation.
//
// The general effect of received Trap and Inform packets do not differ for the
// receiver, and the response is handled by the caller of the handler, so there
// is no need for the application to handle Informs any different than Traps.
// Nonetheless, the packet's Type field can be examined to determine what type
// of event this is for e.g. statistics gathering functions, etc.
type TrapHandlerFunc func(s *SnmpPacket, u *net.UDPAddr)

// NewTrapListener returns an initialized TrapListener.
//
// NOTE: the trap code is currently unreliable when working with snmpv3 - pull requests welcome
func NewTrapListener() *TrapListener {
	tl := &TrapListener{
		finish: 0,
		done:   make(chan bool),
		// Buffered because one doesn't have to block on it.
		listening: make(chan bool, 1),
	}

	return tl
}

// Listening returns a sentinel channel on which one can block
// until the listener is ready to receive requests.
//
// NOTE: the trap code is currently unreliable when working with snmpv3 - pull requests welcome
func (t *TrapListener) Listening() <-chan bool {
	t.Lock()
	defer t.Unlock()
	return t.listening
}

// Close terminates the listening on TrapListener socket
//
// NOTE: the trap code is currently unreliable when working with snmpv3 - pull requests welcome
func (t *TrapListener) Close() {
	// Prevent concurrent calls to Close
	if atomic.CompareAndSwapInt32(&t.finish, 0, 1) {
		// TODO there's bugs here
		if t.conn == nil {
			return
		}
		if t.conn.LocalAddr().Network() == udp {
			t.conn.Close()
		}
		<-t.done
	}
}

func (t *TrapListener) listenUDP(addr string) error {
	// udp

	udpAddr, err := net.ResolveUDPAddr(t.proto, addr)
	if err != nil {
		return err
	}
	t.conn, err = net.ListenUDP(udp, udpAddr)
	if err != nil {
		return err
	}

	defer t.conn.Close()

	// Mark that we are listening now.
	t.listening <- true

	for {
		switch {
		case atomic.LoadInt32(&t.finish) == 1:
			t.done <- true
			return nil

		default:
			var buf [4096]byte
			rlen, remote, err := t.conn.ReadFromUDP(buf[:])
			if err != nil {
				if atomic.LoadInt32(&t.finish) == 1 {
					// err most likely comes from reading from a closed connection
					continue
				}
				t.Params.logPrintf("TrapListener: error in read %s\n", err)
				continue
			}

			msg := buf[:rlen]
			traps := t.Params.UnmarshalTrap(msg, false)

			if traps != nil {
				// Here we assume that t.OnNewTrap will not alter the contents
				// of the PDU (per documentation, because Go does not have
				// compile-time const checking).  We don't pass a copy because
				// the SnmpPacket type is somewhat large, but we could without
				// violating any implicit or explicit spec.
				t.OnNewTrap(traps, remote)

				// If it was an Inform request, we need to send a response.
				if traps.PDUType == InformRequest { //nolint:whitespace

					// Reuse the packet, since we're supposed to send it back
					// with the exact same variables unless there's an error.
					// Change the PDUType to the response, though.
					traps.PDUType = GetResponse

					// If the response can be sent, the error-status is
					// supposed to be set to noError and the error-index set to
					// zero.
					traps.Error = NoError
					traps.ErrorIndex = 0

					// TODO: Check that the message marshalled is not too large
					// for the originator to accept and if so, send a tooBig
					// error PDU per RFC3416 section 4.2.7.  This maximum size,
					// however, does not have a well-defined mechanism in the
					// RFC other than using the path MTU (which is difficult to
					// determine), so it's left to future implementations.
					ob, err := traps.marshalMsg()
					if err != nil {
						return fmt.Errorf("error marshaling INFORM response: %v", err)
					}

					// Send the return packet back.
					count, err := t.conn.WriteTo(ob, remote)
					if err != nil {
						return fmt.Errorf("error sending INFORM response: %v", err)
					}

					// This isn't fatal, but should be logged.
					if count != len(ob) {
						t.Params.logPrintf("Failed to send all bytes of INFORM response!\n")
					}
				}
			}
		}
	}
}

func (t *TrapListener) handleTCPRequest(conn net.Conn) {
	// Make a buffer to hold incoming data.
	buf := make([]byte, 4096)
	// Read the incoming connection into the buffer.
	reqLen, err := conn.Read(buf)
	if err != nil {
		t.Params.logPrintf("TrapListener: error in read %s\n", err)
		return
	}

	msg := buf[:reqLen]
	traps := t.Params.UnmarshalTrap(msg, false)

	if traps != nil {
		// TODO: lying for backward compatibility reason - create UDP Address ... not nice
		r, _ := net.ResolveUDPAddr("", conn.RemoteAddr().String())
		t.OnNewTrap(traps, r)
	}
	// Close the connection when you're done with it.
	conn.Close()
}

func (t *TrapListener) listenTCP(addr string) error {
	tcpAddr, err := net.ResolveTCPAddr(t.proto, addr)
	if err != nil {
		return err
	}

	l, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return err
	}

	defer l.Close()

	// Mark that we are listening now.
	t.listening <- true

	for {
		switch {
		case atomic.LoadInt32(&t.finish) == 1:
			t.done <- true
			return nil
		default:

			// Listen for an incoming connection.
			conn, err := l.Accept()
			fmt.Printf("ACCEPT: %s", conn)
			if err != nil {
				fmt.Println("error accepting: ", err.Error())
				return err
			}
			// Handle connections in a new goroutine.
			go t.handleTCPRequest(conn)
		}
	}
}

// Listen listens on the UDP address addr and calls the OnNewTrap
// function specified in *TrapListener for every trap received.
//
// NOTE: the trap code is currently unreliable when working with snmpv3 - pull requests welcome
func (t *TrapListener) Listen(addr string) error {
	if t.Params == nil {
		t.Params = Default
	}

	// TODO TODO returning an error cause the following to hang/break
	// TestSendTrapBasic
	// TestSendTrapWithoutWaitingOnListen
	// TestSendV1Trap
	_ = t.Params.validateParameters()

	if t.OnNewTrap == nil {
		t.OnNewTrap = t.debugTrapHandler
	}

	splitted := strings.SplitN(addr, "://", 2)
	t.proto = udp
	if len(splitted) > 1 {
		t.proto = splitted[0]
		addr = splitted[1]
	}

	if t.proto == "tcp" {
		return t.listenTCP(addr)
	} else if t.proto == udp {
		return t.listenUDP(addr)
	}

	return fmt.Errorf("not implemented network protocol: %s [use: tcp/udp]", t.proto)
}

// Default trap handler
func (t *TrapListener) debugTrapHandler(s *SnmpPacket, u *net.UDPAddr) {
	t.Params.logPrintf("got trapdata from %+v: %+v\n", u, s)
}

// UnmarshalTrap unpacks the SNMP Trap.
//
// NOTE: the trap code is currently unreliable when working with snmpv3 - pull requests welcome
func (x *GoSNMP) UnmarshalTrap(trap []byte, useResponseSecurityParameters bool) (result *SnmpPacket) {
	result = new(SnmpPacket)

	if x.SecurityParameters != nil {
		err := x.SecurityParameters.initSecurityKeys()
		if err != nil {
			return nil
		}
		result.SecurityParameters = x.SecurityParameters.Copy()
	}

	cursor, err := x.unmarshalHeader(trap, result)
	if err != nil {
		x.logPrintf("UnmarshalTrap: %s\n", err)
		return nil
	}

	if result.Version == Version3 {
		if result.SecurityModel == UserSecurityModel {
			err = x.testAuthentication(trap, result, useResponseSecurityParameters)
			if err != nil {
				x.logPrintf("UnmarshalTrap v3 auth: %s\n", err)
				return nil
			}
		}

		trap, cursor, err = x.decryptPacket(trap, cursor, result)
		if err != nil {
			x.logPrintf("UnmarshalTrap v3 decrypt: %s\n", err)
			return nil
		}
	}
	err = x.unmarshalPayload(trap, cursor, result)
	if err != nil {
		x.logPrintf("UnmarshalTrap: %s\n", err)
		return nil
	}
	return result
}
