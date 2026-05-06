"""
One-shot script to patch src/agentix/bridge/bridge.py with Phase 2 methods.
Run from project root: python _patch_bridge.py
"""

import sys

BRIDGE = "src/agentix/bridge/bridge.py"

# ── New helper methods block (inserted before the module-level create_bridge) ─

NEW_METHODS = '''
    # ── Hierarchical task execution helpers (Phase 2) ─────────────────────────

    def _load_prompt_file(self, name: str) -> "Optional[str]":
        """Load a system prompt by name (without extension) from SYSTEM_PROMPTS_DIR."""
        try:
            from agentix.constants import SYSTEM_PROMPTS_DIR

            matches = glob.glob(f"{SYSTEM_PROMPTS_DIR}{name}.*")
            if not matches:
                return None
            with open(matches[0], "r", encoding="utf-8") as fh:
                return fh.read()
        except Exception:
            return None

    @staticmethod
    def _extract_plan_json(raw: str) -> dict:
        """Extract a JSON object from an LLM response that may contain surrounding text."""
        raw = raw.strip()
        try:
            return json.loads(raw)
        except json.JSONDecodeError:
            pass
        start = raw.find("{")
        end = raw.rfind("}")
        if start != -1 and end > start:
            try:
                return json.loads(raw[start : end + 1])
            except json.JSONDecodeError:
                pass
        raise ValueError(f"No valid JSON found in planner response (length={len(raw)})")

    def _create_plan(
        self,
        prompt: str,
        context: "Context",
        session_plan_index: int = 0,
    ) -> "tuple[Optional[PlanRecord], Optional[TaskTree]]":
        """
        Call the planner LLM to produce a structured plan, then persist it.

        Returns (PlanRecord, TaskTree) on success, (None, None) on failure.
        """
        planner_text = self._load_prompt_file("planner_prompt")
        if not planner_text:
            logger.warning("planner_prompt.md not found; falling back to tool loop")
            return None, None

        history = self._context_to_history(context)
        messages: list[dict] = [msg.to_llm_dict() for msg in history]
        messages.insert(0, {"role": "system", "content": planner_text})
        messages.append({"role": "user", "content": prompt})

        raw_content = ""
        for chunk in self._iter_llm_chunks(messages, tools=None):
            if chunk.type == ChunkType.CONTENT:
                raw_content += chunk.content

        try:
            plan_data = self._extract_plan_json(raw_content)
        except (ValueError, json.JSONDecodeError) as exc:
            logger.warning("Planner JSON parse failed: %s", exc)
            return None, None

        plan_name = plan_data.get("plan_name", f"Plan {session_plan_index + 1}")
        steps_data = plan_data.get("steps", [])

        if not steps_data:
            logger.warning("Planner returned empty steps list")
            return None, None

        steps = [
            PlanStep(
                step_id=s.get("id", f"step_{i}"),
                description=s.get("description") or str(s.get("inputs", "")),
                tbd=s.get("tbd", False),
                depends_on=s.get("depends_on", []),
            )
            for i, s in enumerate(steps_data)
        ]

        plan_id = f"plan_{uuid.uuid4().hex[:8]}"
        plan = PlanRecord(
            plan_id=plan_id,
            plan_name=plan_name,
            session_plan_index=session_plan_index,
            steps=steps,
            root_task_ids=[s.step_id for s in steps if not s.depends_on],
            status="pending",
            epoch=time.time(),
        )

        task_tree = TaskTree(
            session_id=context.session_id or "",
            created_epoch=time.time(),
            last_updated_epoch=time.time(),
        )
        task_tree.add_plan(plan)

        try:
            context.save_plan(plan)
            context.save_task_tree(task_tree)
        except Exception as exc:
            logger.debug("Could not persist plan: %s", exc)

        return plan, task_tree

    def _resolve_tbd_step(
        self,
        step: "PlanStep",
        dep_syntheses: "list[str]",
        plan_name: str,
    ) -> str:
        """Ask the LLM to resolve a TBD step using prerequisite syntheses."""
        context_block = "\\n".join(
            f"Result from {dep}: {synth}" for dep, synth in zip(step.depends_on, dep_syntheses)
        )
        resolve_prompt = (
            f"You are executing plan \\'{plan_name}\\'."
            f" Step \\'{step.step_id}\\' is TBD. Its placeholder: \\"{step.description}\\""
            f"\\n\\nPrerequisite results:\\n{context_block}"
            "\\n\\nWrite a single concrete description (<=15 words) for what this step should do."
            " Output ONLY the description string — no JSON, no explanation."
        )
        resolved = ""
        for chunk in self._iter_llm_chunks([{"role": "user", "content": resolve_prompt}], tools=None):
            if chunk.type == ChunkType.CONTENT:
                resolved += chunk.content
        return resolved.strip().strip(\'"\').strip("\'") or step.description

    def _run_task_node(
        self,
        *,
        plan_id: str,
        task_id: str,
        task_description: str,
        parent_task_id: "Optional[str]" = None,
        depth: int = 0,
        plan_step_index: int = 0,
        tbd: bool = False,
        context: "Context",
        task_tree: "TaskTree",
        initial_messages: "list[dict]",
        max_rounds: int = 10,
    ) -> "Iterator[ResponseChunk]":
        """Execute a single task node, recursing into sub-tasks via run_subtask."""
        max_task_depth: int = getattr(self.config, "max_task_depth", 10)

        yield ResponseChunk(
            type=ChunkType.TASK_NODE_START,
            plan_id=plan_id,
            task_id=task_id,
            parent_task_id=parent_task_id,
            task_depth=depth,
            tbd=tbd,
            content=task_description,
        )

        node = TaskNodeRecord(
            plan_id=plan_id,
            task_id=task_id,
            parent_task_id=parent_task_id,
            depth=depth,
            plan_step_index=plan_step_index,
            task_description=task_description,
            tbd=tbd,
            status="running",
            epoch=time.time(),
        )
        try:
            context.save_task_node(node)
            task_tree.add_node(node)
            context.save_task_tree(task_tree)
        except Exception as exc:
            logger.debug("Could not persist task node %s: %s", task_id, exc)

        # Build messages with task_execution prompt injected at position 0
        messages: list[dict] = list(initial_messages)
        task_exec_text = self._load_prompt_file("task_execution")
        if task_exec_text:
            task_exec_text = task_exec_text.replace("{depth}", str(depth)).replace(
                "{max_depth}", str(max_task_depth)
            )
            messages.insert(0, {"role": "system", "content": task_exec_text})
        if not messages or messages[-1].get("content") != task_description:
            messages.append({"role": "user", "content": task_description})

        available_tools = self.get_available_tools()
        if depth >= max_task_depth:
            available_tools = [
                t for t in available_tools
                if t.get("function", {}).get("name") != "run_subtask"
            ]

        any_tools_called = False
        got_content = False
        synthesis_text = ""

        for round_index in range(max_rounds):
            tool_calls_this_round: list[ResponseChunk] = []
            content_chunks: list[ResponseChunk] = []

            for chunk in self._iter_llm_chunks(messages, tools=available_tools or None):
                if chunk.type == ChunkType.TOOL_CALL:
                    tool_calls_this_round.append(chunk)
                elif chunk.type in (ChunkType.CONTENT, ChunkType.THINKING):
                    content_chunks.append(chunk)
                    yield chunk
                    if chunk.type == ChunkType.CONTENT and chunk.content:
                        got_content = True
                        synthesis_text += chunk.content
                elif chunk.type == ChunkType.DONE:
                    pass
                else:
                    yield chunk

            if not tool_calls_this_round:
                break

            any_tools_called = True
            got_content = False
            synthesis_text = ""

            assistant_msg: dict = {
                "role": "assistant",
                "content": "".join(c.content for c in content_chunks),
                "tool_calls": [
                    {
                        "id": tc.tool_id or f"call_{i}",
                        "type": "function",
                        "function": {
                            "name": tc.tool_name,
                            "arguments": json.dumps(tc.tool_input or {}),
                        },
                    }
                    for i, tc in enumerate(tool_calls_this_round)
                ],
            }
            messages.append(assistant_msg)

            subtask_calls = [tc for tc in tool_calls_this_round if tc.tool_name == "run_subtask"]
            regular_calls = [tc for tc in tool_calls_this_round if tc.tool_name != "run_subtask"]
            tool_result_messages: list[dict] = []

            for tc in subtask_calls:
                yield ResponseChunk(
                    type=ChunkType.TOOL_CALL,
                    tool_name=tc.tool_name,
                    tool_input=tc.tool_input,
                    tool_id=tc.tool_id,
                    round_index=round_index,
                    plan_id=plan_id,
                    task_id=task_id,
                )
                sub_args = tc.tool_input or {}
                sub_task_id = f"subtask_{uuid.uuid4().hex[:8]}"
                sub_description = sub_args.get("task", "")
                sub_synthesis = ""
                for sub_chunk in self._run_task_node(
                    plan_id=plan_id,
                    task_id=sub_task_id,
                    task_description=sub_description,
                    parent_task_id=task_id,
                    depth=depth + 1,
                    plan_step_index=0,
                    tbd=False,
                    context=context,
                    task_tree=task_tree,
                    initial_messages=[],
                    max_rounds=max_rounds,
                ):
                    yield sub_chunk
                    if sub_chunk.type == ChunkType.TASK_NODE_END:
                        sub_synthesis = sub_chunk.content or ""
                node.child_task_ids.append(sub_task_id)
                yield ResponseChunk(
                    type=ChunkType.TOOL_RESULT,
                    tool_name=tc.tool_name,
                    tool_output=sub_synthesis,
                    tool_id=tc.tool_id,
                    round_index=round_index,
                    plan_id=plan_id,
                    task_id=task_id,
                )
                tool_result_messages.append({
                    "role": "tool",
                    "tool_call_id": tc.tool_id or f"call_{sub_task_id}",
                    "content": sub_synthesis,
                })

            if regular_calls:
                tc_list = list(regular_calls)
                with ThreadPoolExecutor(max_workers=min(len(regular_calls), 4)) as pool:
                    futures = {
                        pool.submit(self.execute_tool, tc.tool_name, tc.tool_input or {}, tc.tool_id): tc
                        for tc in regular_calls
                    }
                    for future in as_completed(futures):
                        tc = futures[future]
                        yield ResponseChunk(
                            type=ChunkType.TOOL_CALL,
                            tool_name=tc.tool_name,
                            tool_input=tc.tool_input,
                            tool_id=tc.tool_id,
                            round_index=round_index,
                            plan_id=plan_id,
                            task_id=task_id,
                        )
                        result: ToolResponse = future.result()
                        yield ResponseChunk(
                            type=ChunkType.TOOL_RESULT,
                            tool_name=tc.tool_name,
                            tool_output=result.output if result.success else result.error,
                            tool_id=tc.tool_id,
                            round_index=round_index,
                            plan_id=plan_id,
                            task_id=task_id,
                        )
                        tool_result_messages.append({
                            "role": "tool",
                            "tool_call_id": tc.tool_id or f"call_{tc_list.index(tc)}",
                            "content": result.to_llm_format(),
                        })

            messages.extend(tool_result_messages)

        # Guaranteed synthesis when tools were used but no final text was produced
        if any_tools_called and not got_content:
            synthesis_messages = messages + [
                {
                    "role": "user",
                    "content": (
                        "You have gathered sufficient information through your tool calls. "
                        "Now write your complete synthesis for this task. "
                        "Do not call any more tools."
                    ),
                }
            ]
            for chunk in self._iter_llm_chunks(synthesis_messages):
                if chunk.type == ChunkType.TOOL_CALL:
                    continue
                if chunk.type == ChunkType.DONE:
                    continue
                yield chunk
                if chunk.type == ChunkType.CONTENT and chunk.content:
                    synthesis_text += chunk.content
                    got_content = True
            if not synthesis_text:
                synthesis_text = "(no synthesis produced)"

        node.status = "done"
        node.synthesis_epoch = time.time()
        try:
            context.save_task_node(node)
            task_tree.nodes[task_id] = node
            context.save_task_tree(task_tree)
        except Exception as exc:
            logger.debug("Could not persist completed task node %s: %s", task_id, exc)

        yield ResponseChunk(
            type=ChunkType.TASK_NODE_END,
            plan_id=plan_id,
            task_id=task_id,
            parent_task_id=parent_task_id,
            task_depth=depth,
            content=synthesis_text,
        )

    def _run_plan(
        self,
        plan: "PlanRecord",
        context: "Context",
        task_tree: "TaskTree",
        base_messages: "list[dict]",
        original_prompt: str,
    ) -> "Iterator[ResponseChunk]":
        """Execute all plan steps, resolving TBD steps at runtime, then synthesise."""
        max_rounds: int = getattr(self.config, "max_tool_rounds", 10)
        step_syntheses: dict[str, str] = {}

        for i, step in enumerate(plan.steps):
            # Skip steps whose dependencies are not yet satisfied
            unsatisfied = [d for d in step.depends_on if d not in step_syntheses]
            if unsatisfied:
                logger.warning(
                    "Step %s deps not yet completed: %s; skipping",
                    step.step_id, unsatisfied,
                )
                continue

            effective_description = step.description

            # Resolve TBD step description using predecessor syntheses
            if step.tbd:
                dep_synths = [step_syntheses.get(d, "") for d in step.depends_on]
                resolved = self._resolve_tbd_step(step, dep_synths, plan.plan_name)
                effective_description = resolved
                yield ResponseChunk(
                    type=ChunkType.TASK_NODE_TBD,
                    plan_id=plan.plan_id,
                    task_id=step.step_id,
                    content=resolved,
                )
                try:
                    context.save_plan(plan)
                except Exception:
                    pass

            # Build per-step message set (inject dep context if present)
            step_messages = list(base_messages)
            if step.depends_on:
                dep_context = "\\n".join(
                    f"Result from {dep}: {step_syntheses[dep]}"
                    for dep in step.depends_on
                    if dep in step_syntheses
                )
                if dep_context:
                    step_messages.append({
                        "role": "system",
                        "content": f"Context from completed prerequisite steps:\\n{dep_context}",
                    })

            step_synthesis = ""
            for chunk in self._run_task_node(
                plan_id=plan.plan_id,
                task_id=step.step_id,
                task_description=effective_description,
                parent_task_id=None,
                depth=0,
                plan_step_index=i,
                tbd=step.tbd,
                context=context,
                task_tree=task_tree,
                initial_messages=step_messages,
                max_rounds=max_rounds,
            ):
                yield chunk
                if chunk.type == ChunkType.TASK_NODE_END and chunk.task_id == step.step_id:
                    step_synthesis = chunk.content or ""

            step_syntheses[step.step_id] = step_synthesis

        # Final cross-step synthesis emitted as regular CONTENT stream
        if step_syntheses:
            combined = "\\n\\n".join(
                f"Step {sid}:\\n{synth}" for sid, synth in step_syntheses.items()
            )
            final_messages = list(base_messages) + [
                {"role": "user", "content": original_prompt},
                {
                    "role": "assistant",
                    "content": (
                        "I have gathered the following information through my research:\\n\\n"
                        + combined
                    ),
                },
                {
                    "role": "user",
                    "content": (
                        "Based on all the information above, please provide your complete "
                        "and final answer to my original request."
                    ),
                },
            ]
            for chunk in self._iter_llm_chunks(final_messages):
                if chunk.type == ChunkType.TOOL_CALL:
                    continue
                if chunk.type == ChunkType.DONE:
                    continue
                yield chunk

'''

# ── Updated _stream_planned_response body ─────────────────────────────────────

NEW_STREAM_PLANNED = '''    def _stream_planned_response(
        self,
        prompt: str,
        context: Context,
    ) -> Iterator[ResponseChunk]:
        """
        Stream a multi-step planned response using the hierarchical task engine.

        Calls the planner LLM to obtain a structured plan then executes each
        step via _run_task_node (depth 0), forwarding all emitted chunks to the
        caller.  Falls back silently to _run_tool_loop if the planner fails.
        """
        try:
            existing_plans = context.load_plans() or []
        except Exception:
            existing_plans = []

        plan, task_tree = self._create_plan(prompt, context, session_plan_index=len(existing_plans))

        if plan is None or task_tree is None:
            max_rounds = getattr(self.config, "max_tool_rounds", 10)
            yield from self._run_tool_loop(prompt, context, max_rounds=max_rounds)
            return

        yield ResponseChunk(
            type=ChunkType.PLAN_START,
            plan_id=plan.plan_id,
            plan_name=plan.plan_name,
            content=plan.plan_name,
        )

        history = self._context_to_history(context)
        base_messages: list[dict] = [msg.to_llm_dict() for msg in history]
        try:
            from agentix.context.prompts import get_system_prompt

            sys_content = get_system_prompt(self.config)
            if sys_content:
                base_messages.insert(0, {"role": "system", "content": sys_content})
        except Exception:
            pass

        plan.status = "running"
        try:
            context.save_plan(plan)
        except Exception:
            pass

        yield from self._run_plan(plan, context, task_tree, base_messages, prompt)

        plan.status = "done"
        try:
            context.save_plan(plan)
        except Exception:
            pass

        yield ResponseChunk(type=ChunkType.DONE, done_reason="stop")

'''

# ── Module-level run_subtask function ─────────────────────────────────────────

RUN_SUBTASK_FN = '''
def run_subtask(task: str, scratch_file: Optional[str] = None) -> str:
    """Request execution of a focused sub-task with its own bounded tool loop.

    Creates a child task node that runs within a fresh context window using the
    task_execution system prompt. On completion its synthesis (50-200 words) is
    returned as the result of this tool call.

    Use run_subtask when:
    - Investigation of a sub-problem would pollute the current context window.
    - A focused, bounded scope produces a cleaner result than continued tool calls.
    - Intermediate results need to be passed via a scratch file.

    Do NOT use run_subtask for simple tool calls — call the tool directly instead.

    Args:
        task: Complete, self-contained description of the sub-task including
              relevant file paths, scope, and success criteria. The sub-task has
              NO access to the parent task\'s context window.
        scratch_file: Optional relative path within the session scratch directory
                      for passing large intermediate data between tasks.

    Returns:
        Synthesis text (50-200 words) summarising the sub-task result.
    """
    raise NotImplementedError(
        "run_subtask must be intercepted by _run_task_node\'s tool loop before "
        "execute_tool is reached. This function exists solely for schema extraction."
    )

'''

# ── Read current file ─────────────────────────────────────────────────────────

with open(BRIDGE, "r") as f:
    src = f.read()

# ── Step 1: Replace _stream_planned_response ──────────────────────────────────

OLD_STREAM = """\
    def _stream_planned_response(
        self,
        prompt: str,
        context: Context,
    ) -> Iterator[ResponseChunk]:
        \"\"\"
        Stream a multi-step planned response using the tool loop.

        Uses the full configurable max_rounds so the LLM can chain multiple
        tool calls before producing a final answer.

        Args:
            prompt: User prompt
            context: Conversation context

        Yields:
            Planning, tool call, tool result, and content chunks.
        \"\"\"
        max_rounds = getattr(self.config, "max_tool_rounds", 10)
        yield from self._run_tool_loop(prompt, context, max_rounds=max_rounds)"""

if OLD_STREAM not in src:
    print("ERROR: Could not find old _stream_planned_response body")
    sys.exit(1)

src = src.replace(OLD_STREAM, NEW_STREAM_PLANNED.rstrip("\n"), 1)
print("Step 1 done: _stream_planned_response replaced")

# ── Step 2: Insert new class methods before _run_tool_loop ────────────────────

# Find the line "    def _run_tool_loop(" and insert the new block before it
TOOL_LOOP_DEF = "\n    def _run_tool_loop(\n"
insert_pos = src.find(TOOL_LOOP_DEF)
if insert_pos == -1:
    print("ERROR: Could not find _run_tool_loop definition")
    sys.exit(1)

src = src[:insert_pos] + NEW_METHODS + src[insert_pos:]
print("Step 2 done: new class methods inserted before _run_tool_loop")

# ── Step 3: Add module-level run_subtask before create_bridge ─────────────────

BRIDGE_MARKER = "\n# Convenience function for quick usage\ndef create_bridge("
pos = src.find(BRIDGE_MARKER)
if pos == -1:
    print("ERROR: Could not find create_bridge marker")
    sys.exit(1)

src = src[:pos] + "\n" + RUN_SUBTASK_FN + src[pos:]
print("Step 3 done: run_subtask function added before create_bridge")

# ── Write ─────────────────────────────────────────────────────────────────────

with open(BRIDGE, "w") as f:
    f.write(src)

print(f"Done. Total lines: {src.count(chr(10))}")
