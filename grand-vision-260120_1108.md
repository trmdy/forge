# grand vision

## forge taxonomy

AGENTS dot md

<u>profile (harness)</u>

- codex

- claude code

- pi

- opencode

- amp

- droid

PROMPT dot md

**autodetection**

**loop**

- id

- name

- ledger

- seq

agent+prompt

**workflow run**

- id

- name

- steps

- current step

- ledger

bash

**step**

**workflow**

other command

logic (if, branching, etc)

job

node

- 1 master

- multiple non-masters

mesh

trigger

- remote call

- one off

- cron job

## loops

A loop is "ralph" agent loop. Its primary purpose is to repeatedly do a task.

**CLI spec**

- forge loop = flp

- flp up

- flp up --alias cmd

- flp up --pool <pool>

- flp ps

- flp rm

- flp stop

- flp resume

- flp send

## workflow

A forge workflow is a directed graph of steps. Each step may be a agent run with a prompt, some code, a bash command, or some other step. Every single run might also have pre-hooks and post-hooks.

A workflow is self-contained in a toml file and may be shared. This allows complex development flows to be contained in reusable and shareable structures.

Workflows may contain loops (i.e. ralph loops) which run a set amount of times or an infinite amount of times. Loops may have stop conditions that are concrete (num loops, result of a tool call, no remaining tasks, etc), or which require LLM evaluations.

Workflows may contain fan outs and stop/continue conditions. For instance, a workflow may have a linear step sequence for the initial specification, say a task creation round with 2-3 steps, followed by a fanout to 5 agents actually implementing the specification until all tasks are marked as complete, after which we go back to a linear QA flow. See the diagram to the right.

Steps in workflows may depend on and be alive solely due to other steps existing (parasitic steps). This is useful for instance when you want a commiter agent running once every 5 minutes as long as ralph loop agent is running.

Workflows may also contain human in the loop moments, where a response or check from a human has to be entered.

## Workflow example

start

\[agent\]: Read PRD and create tasks 

stop condition: Count\[tasks\] > 0

\[agent\]: Review tasks and break down further

stop condition: LLM review of coverage

## Loops

\[agent\]: Pick a task and implement

stop cond: Count\[tasks\[open\]\] == 0

\[agent\]: Pick a task and implement

stop cond: Count\[tasks\[open\]\] == 0

\[agent\]: Pick a task and implement

stop cond: Count\[tasks\[open\]\] == 0

\[agent\]: Pick a task and implement

stop cond: Count\[tasks\[open\]\] == 0

\[agent\]: Do deep code review. write to qa.md

\[agent\]: Create detailed tasks from qa.md

## Fan out if many tasks

if tasks > 20

else

## Loops

\[agent\]: Pick a task and implement

stop cond: Count\[tasks\[open\]\] == 0

\[agent\]: Pick a task and implement

stop cond: Count\[tasks\[open\]\] == 0

\[agent\]: Pick a task and implement

stop cond: Count\[tasks\[open\]\] == 0

\[agent\]: design review, create tasks

\[agent\]: fix design review tasks

\[agent\]: commit work and create PR on github

## node

A node in forge is a registered computer where at least the following are true:

1. forge is installed

2. forged is running and can accept commands.

Registered nodes are used to relay forge commands to another computer. This makes it possible for us to delegate work to agents on that computer.

Communication to forge happens over HTTPS with a bearer token.

## mesh

The mesh is the list of all your nodes. When you setup a new node you may join a mesh. When you init forge a mesh config is created for you automatically. 

Your local config supports changing between meshes, but a node (computer) can only be a part of one mesh at a time.

A mesh has one and only one active master. The master node is responsible for coordinating tasks and sending commands when there are many nodes. 

forge works in a client-server model where you communicate primarily with the master node which sends your commands to the relevant nodes.

If the master node falls out a new master node can automatically be resolved.

## job

A job in forge is something that completes some defined work. It may spawn a workflow, multiple in parallell, in sequence, run bash scripts, or do something else.

A job may spawn work on multiple nodes, alter forge itself / do other meta work. 

A job has input and and output. Both may be null.

A job has a trigger.

## trigger

A trigger is something that starts a job. 

Either a direct start (cli), a cronjob or a http trigger (webhook).