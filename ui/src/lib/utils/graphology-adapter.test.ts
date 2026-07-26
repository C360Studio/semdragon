import { MultiDirectedGraph } from 'graphology';
import { describe, expect, it } from 'vitest';
import { parseEntityId, type GraphEntity } from '$lib/api/graph-types';
import { syncStoreToGraph } from './graphology-adapter';

describe('graphology adapter labels', () => {
	it('uses the canonical agent display-name predicate', () => {
		const id = 'c360.prod.game.board1.agent.code-forger';
		const entity: GraphEntity = {
			id,
			idParts: parseEntityId(id),
			properties: [
				{
					predicate: 'agent.identity.name',
					object: 'CodeForger',
					confidence: 1,
					source: 'test',
					timestamp: 0
				},
				{
					predicate: 'agent.identity.display-name',
					object: 'The Code Forger',
					confidence: 1,
					source: 'test',
					timestamp: 0
				}
			],
			outgoing: [],
			incoming: []
		};
		const graph = new MultiDirectedGraph();

		syncStoreToGraph(graph, [entity], []);

		expect(graph.getNodeAttribute(id, 'label')).toBe('The Code Forger');
	});
});
