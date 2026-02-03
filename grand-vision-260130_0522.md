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

agent

team

job

node

- 1 master

- multiple non-masters

mesh

trigger

- remote call

- one off

- cron job

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

## team

A team is a predefined set of agents that have a specific system prompt, ready to take on tasks.

A team is used to have an "always-ready" set of agents available to do work in a project. Teams typically communicate via fmail when working.

Teams differ from workflows. Workflows are used to perform a linear set of steps, dependent on some trigger. When the workflow is done, the agents disappear. Agents in a workflow are ephemeral.

Teams are persistent. Agents in teams have IDs and names that live long. They have multiple executions and memories. They are "agents" in a more literal way.

Agents in teams can be viewed more like  employees, and teams more like a department, team or company.

Teams and agents in teams may employ workflows. This is done on a per-agent basis. 

<small>This copy was made due to a syncing conflict</small>

**Task delegation**

Sending tasks to a team can be done in multiple ways.

Teams may receive tasks from webhooks and cron jobs, like jobs.

You can also send tasks directly to individual agents via the forge cli, or the forge UI.

You can send tasks directly to a team as a whole. The task will then be delegated automatically inside the team. How tasks are delegated can be configured by rules.

Tasks can either be as simple as single message strings, but also arbitrary json objects. How these json objects are interpreted by the team and how they are delegated is configurable in rules. 

For instance, you might have a "Manager" agent that by default receives all message. However, if the message contains a "type": "design" key-value-pair, the message is forwarded to the "Designer" agent.

**Heartbeats and always-on-teams**

One of the key advantages of our teams systematisation and our task triggers is that we can create "always-on" swarms that continuously process and improve tasks.

By having a task ingestion system tied towards Linear, Slack or other locations we do work, our team is ready to accept arbitrary tasks and handle them appropriately.

One might, for instance, have a swarm in a repo that is connected

**Leaders**

Teams may have designated team leaders.

A team leader is designed to orchestrate the other agents. Most agents in a team assume they hold no power of other agents to instruct them what to do. Similarly they don't assume others hold power over them.

When communicating between themselves agents might give each other assistance and suggestions. But they are given no special emphasis to follow "orders".

Unless the agent they receive a message from is a team leader. They they will to a much larger extent follow orders.

## job

A job in forge is something that completes some defined work. It may spawn a workflow, multiple in parallell, in sequence, run bash scripts, or do something else.

A job may spawn work on multiple nodes, alter forge itself / do other meta work. 

A job has input and and output. Both may be null.

A job has a trigger.

## trigger

A trigger is something that starts a job. 

Either a direct start (cli), a cronjob or a http trigger (webhook).

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

## workflow

A forge workflow is a directed graph of steps. Each step may be a agent run with a prompt, some code, a bash command, or some other step. Every single run might also have pre-hooks and post-hooks.

A workflow is self-contained in a toml file and may be shared. This allows complex development flows to be contained in reusable and shareable structures.

Workflows may contain loops (i.e. ralph loops) which run a set amount of times or an infinite amount of times. Loops may have stop conditions that are concrete (num loops, result of a tool call, no remaining tasks, etc), or which require LLM evaluations.

Workflows may contain fan outs and stop/continue conditions. For instance, a workflow may have a linear step sequence for the initial specification, say a task creation round with 2-3 steps, followed by a fanout to 5 agents actually implementing the specification until all tasks are marked as complete, after which we go back to a linear QA flow. See the diagram to the right.

Steps in workflows may depend on and be alive solely due to other steps existing (parasitic steps). This is useful for instance when you want a commiter agent running once every 5 minutes as long as ralph loop agent is running.

Workflows may also contain human in the loop moments, where a response or check from a human has to be entered.

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